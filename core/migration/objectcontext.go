package migration

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	time2 "github.com/anyproto/anytype-heart/util/time"
)

// currentObjectContextMigrationVersion is the version of the object context migration.
// Bump this to force re-migration if issues are found or we started to a migrate all objects
const currentObjectContextMigrationVersion = 10

// contextTimeTolerance is the maximum allowed time difference (in seconds) between a file's
// creation and a potential context (block link or chat message). This accounts for the fact
// that file objects are created before the containing block/message is finalized.
const contextTimeTolerance = 5 * 60 // 5 minutes

// systemRelationsToSkip contains system relations that should be skipped when building
// the incoming links map, as they are not meaningful for determining file creation context
var systemRelationsToSkip = []domain.RelationKey{
	bundle.RelationKeyCreator,
	bundle.RelationKeyLastModifiedBy,
	bundle.RelationKeyType,
	bundle.RelationKeyBacklinks,
	bundle.RelationKeyResolvedLayout,
	bundle.RelationKeyRecommendedFeaturedRelations,
	bundle.RelationKeyRecommendedRelations,
	bundle.RelationKeyRecommendedHiddenRelations,
	bundle.RelationKeySpaceId,
	bundle.RelationKeyIdentityProfileLink,
	bundle.RelationKeyTargetObjectType,
	bundle.RelationKeySourceObject,
}

/*
Context Migration Logic:

1. Make sure you Wait until all active syncing is done
   - Ensures all objects and their links are indexed before migration (done in spaceIndexer)

2. Find all file objects without the CreatedInContext field set

3. For each file, find creation context:
   - Look up inbound links in anystore (we have indexed lookup for this)
   - Priority: block links (have blockId) over relation links (no blockId)
   - Sort by blockId and pick first - blockId is BSON ObjectId containing timestamp,
     so lexicographic sort gives chronological order (oldest block first = original context)
   - Set CreatedInContext = source objectId, CreatedInContextRef = blockId (if block link)
*/

func (s *service) runObjectContextMigration(ctx context.Context, spaceId string, workspaceId string) error {
	spaceIndex := s.objectStore.SpaceIndex(spaceId)

	var l = log.With(zap.String("spaceId", spaceId), zap.Int("version", currentObjectContextMigrationVersion))
	// Check if migration already done (version >= current)
	if s.isObjectContextMigrationDone(spaceIndex, workspaceId) {
		l.Debug("migration already done")
		return nil
	}

	l.Info("starting object context migration")

	fileLayouts := make([]int64, 0, len(domain.FileLayouts))
	for _, layout := range domain.FileLayouts {
		fileLayouts = append(fileLayouts, int64(layout))
	}

	// Step 1: Query all file and image objects without context fields
	fileRecords, err := spaceIndex.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(fileLayouts),
			},
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_Empty,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to query files without context: %w", err)
	}

	l.With(zap.Int("count", len(fileRecords))).Debug("found files without context")

	if len(fileRecords) == 0 {
		return nil
	}

	// Step 2: Build chat attachment index (single scan of all chats)
	chatAttachmentIndex, err := s.buildChatAttachmentIndex(ctx, spaceId, spaceIndex)
	if err != nil {
		l.Warn("failed to build chat attachment index", zap.Error(err))
		// Continue without chat context - not fatal
		chatAttachmentIndex = make(ChatAttachmentIndex)
	}
	l.Debug("built chat attachment index", zap.Int("attachments", len(chatAttachmentIndex)))

	// Step 3: Process each file - use indexed lookup for inbound links
	migratedCount := 0
	for _, fileRecord := range fileRecords {
		fileId := fileRecord.Details.GetString(bundle.RelationKeyId)
		fileObjectCreatedDate := fileRecord.Details.GetInt64(bundle.RelationKeyAddedDate)

		fl := l.With(zap.String("fileId", fileId), zap.Int64("creationDate", fileObjectCreatedDate))
		if fileObjectCreatedDate == 0 {
			fl.Warn("file object has no created date")
			continue
		}

		// Get inbound links for this file using indexed lookup
		inboundLinks, err := spaceIndex.GetInboundLinksDetailedById(fileId)
		if err != nil {
			fl.Warn("failed to get inbound links", zap.Error(err))
			continue
		}

		// Find the best context (block link, chat, or relation fallback)
		chatCtx := chatAttachmentIndex[fileId]
		contextInfo := s.findBestContext(fileObjectCreatedDate, fileId, inboundLinks, chatCtx)
		if contextInfo == nil {
			fl.Debug("no creation context found for file", zap.Int("inboundLinks", len(inboundLinks)), zap.Any("chat", chatCtx))
			continue
		}

		// Verify context object exists and was created before the file
		if err := s.verifyContextObject(spaceIndex, contextInfo, fileObjectCreatedDate); err != nil {
			fl.Warn("context verification failed",
				zap.Error(err),
				zap.String("objectId", contextInfo.objectId),
				zap.String("blockId", contextInfo.blockId),
			)
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// Use detailsService to update the file
		if err := s.detailsService.ModifyDetails(nil, fileId, func(current *domain.Details) (*domain.Details, error) {
			if current.GetString(bundle.RelationKeyCreatedInContext) != "" {
				// double check in case we raced with another user and exit early
				return current, fmt.Errorf("context detail already set(race?)")
			}
			current.Set(bundle.RelationKeyCreatedInContext, domain.String(contextInfo.objectId))
			if contextInfo.blockId != "" {
				current.Set(bundle.RelationKeyCreatedInContextRef, domain.String(contextInfo.blockId))
			} else if contextInfo.messageId != "" {
				current.Set(bundle.RelationKeyCreatedInContextRef, domain.String(contextInfo.messageId))
			}
			return current, nil
		}); err != nil {
			fl.Error("failed to update file context",
				zap.Int("migratedCount", migratedCount),
				zap.Error(err))
			// better to fail fast in this case
			return fmt.Errorf("failed to update file context: %w", err)
		}

		migratedCount++
	}

	l.Info("completed file context migration",
		zap.String("spaceId", spaceId),
		zap.Int("migratedCount", migratedCount),
		zap.Int("totalFiles", len(fileRecords)))

	// Mark migration as done with current version
	if err := s.markFileContextMigrationDone(workspaceId); err != nil {
		l.Warn("failed to mark migration done", zap.Error(err))
	}

	return nil
}

// contextInfo stores the resolved context for a file
type contextInfo struct {
	objectId  string // context object ID (page, chat, etc.)
	blockId   string // block ID for block link contexts
	messageId string // message ID for chat contexts
	timestamp int64  // 0 means no timestamp (relation link fallback)
}

// findBestContext finds the best creation context for a file
// Priority: earliest timed context (block link or chat), then relation link fallback
func (s *service) findBestContext(fileTs int64, fileId string, inboundLinks []spaceindex.IncomingLink, chatCtx *ChatAttachmentContext) *contextInfo {
	// Filter links
	links := s.filterLinks(fileId, inboundLinks)

	// 1. Find earliest block link (has timestamp)
	blockCtx := s.findBlockContext(fileTs, links)

	// 2. Build chat context if valid
	var chatInfo *contextInfo
	if chatCtx != nil {
		// chat message may be created shortly after the file object
		if chatCtx.CreatedAt < fileTs+contextTimeTolerance {
			chatInfo = &contextInfo{
				objectId:  chatCtx.ChatObjectId,
				messageId: chatCtx.MessageId,
				timestamp: chatCtx.CreatedAt,
			}
		}
	}

	// 3. Pick earliest timed context
	if blockCtx != nil && chatInfo != nil {
		if blockCtx.timestamp <= chatInfo.timestamp {
			return blockCtx
		}
		return chatInfo
	}
	if blockCtx != nil {
		return blockCtx
	}
	if chatInfo != nil {
		return chatInfo
	}

	// 4. Fallback to relation link (no timestamp)
	return s.findRelationContext(links)
}

// filterLinks removes self-references and system relations
func (s *service) filterLinks(fileId string, inboundLinks []spaceindex.IncomingLink) []spaceindex.IncomingLink {
	var links []spaceindex.IncomingLink
	for _, link := range inboundLinks {
		if link.SourceID == fileId {
			continue
		}
		if slices.Contains(systemRelationsToSkip, domain.RelationKey(link.RelationKey)) {
			continue
		}
		links = append(links, link)
	}
	return links
}

// findBlockContext finds the earliest valid block link context
func (s *service) findBlockContext(fileTs int64, links []spaceindex.IncomingLink) *contextInfo {
	var earliest contextInfo

	for _, link := range links {
		if link.BlockID == "" {
			continue
		}
		blockTs, ok := time2.BsonIdToTimestamp(link.BlockID)
		if !ok {
			continue
		}
		if blockTs > fileTs+contextTimeTolerance {
			continue
		}
		if earliest.timestamp == 0 || blockTs < earliest.timestamp {
			earliest = contextInfo{
				objectId:  link.SourceID,
				blockId:   link.BlockID,
				timestamp: blockTs,
			}
		}
	}
	// Return nil if no valid block link was found
	if earliest.objectId == "" {
		return nil
	}
	return &earliest
}

// findRelationContext finds the first relation link as fallback (no timestamp)
func (s *service) findRelationContext(links []spaceindex.IncomingLink) *contextInfo {
	// Sort for determinism
	sort.Slice(links, func(i, j int) bool {
		return links[i].RelationKey < links[j].RelationKey
	})

	for _, link := range links {
		if link.BlockID == "" {
			return &contextInfo{
				objectId: link.SourceID,
			}
		}
	}
	return nil
}

func (s *service) isObjectContextMigrationDone(spaceIndex spaceindex.Store, workspaceId string) bool {
	recs, err := spaceIndex.QueryByIds([]string{workspaceId})
	if err != nil || len(recs) == 0 {
		return false
	}
	storedVersion := recs[0].Details.GetInt64(bundle.RelationKeyMigrationObjectContext)
	return storedVersion >= currentObjectContextMigrationVersion
}

// verifyContextObject checks that the context object exists and was created before (or close to) the file object.
// For chat contexts (messageId set), the timestamp was already validated in findBestContext.
func (s *service) verifyContextObject(spaceIndex spaceindex.Store, ci *contextInfo, fileObjectCreatedDate int64) error {
	rec, err := spaceIndex.QueryByIds([]string{ci.objectId})
	if err != nil {
		return fmt.Errorf("failed to query context object: %w", err)
	}
	if len(rec) == 0 {
		return fmt.Errorf("context object not found")
	}
	if ci.messageId == "" {
		createdDate := rec[0].Details.GetInt64(bundle.RelationKeyCreatedDate)
		if createdDate > fileObjectCreatedDate+contextTimeTolerance {
			return fmt.Errorf("context object is newer than file object by %d seconds", createdDate-fileObjectCreatedDate)
		}
	}
	return nil
}

func (s *service) markFileContextMigrationDone(workspaceId string) error {
	return s.detailsService.SetDetails(nil, workspaceId, []domain.Detail{
		{Key: bundle.RelationKeyMigrationObjectContext, Value: domain.Int64(currentObjectContextMigrationVersion)},
	})
}
