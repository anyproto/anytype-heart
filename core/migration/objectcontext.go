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
const currentObjectContextMigrationVersion = 1

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
   - Set CreatedInContext = source objectId, CreatedInBlockId = blockId (if block link)
*/

func (s *service) runObjectContextMigration(ctx context.Context, spaceId string, workspaceId string) error {
	spaceIndex := s.objectStore.SpaceIndex(spaceId)

	var l = log.With("spaceId", spaceId, "version", currentObjectContextMigrationVersion)
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

	l.With("count", len(fileRecords)).Debug("found files without context")

	if len(fileRecords) == 0 {
		return nil
	}

	// Step 2: Process each file - use indexed lookup for inbound links
	migratedCount := 0
	for _, fileRecord := range fileRecords {
		fileId := fileRecord.Details.GetString(bundle.RelationKeyId)
		fileObjectCreatedDate := fileRecord.Details.GetInt64(bundle.RelationKeyAddedDate)
		if fileObjectCreatedDate == 0 {
			log.Warn("file object has no created date", zap.String("fileId", fileId))
			continue
		}

		// Get inbound links for this file using indexed lookup
		inboundLinks, err := spaceIndex.GetInboundLinksDetailedById(fileId)
		if err != nil {
			l.Warn("failed to get inbound links", zap.String("fileId", fileId), zap.Error(err))
			continue
		}

		// Find the creation context for this file
		contextInfo := s.findCreationContext(fileObjectCreatedDate, fileId, inboundLinks)
		if contextInfo == nil {
			l.Debug("no creation context found for file", zap.String("fileId", fileId))
			continue
		}
		rec, err := spaceIndex.QueryByIds([]string{contextInfo.contextId})
		if err != nil {
			l.Warn("failed to query context object", zap.String("fileId", fileId), zap.Error(err))
			continue
		}
		if len(rec) == 0 {
			l.Warn("context object not found", zap.String("fileId", fileId))
			continue
		}

		// double check, source object should be older than file object
		if rec[0].Details.GetInt64(bundle.RelationKeyCreatedDate) > fileObjectCreatedDate {
			l.Warn("context object is newer than file object", zap.String("fileId", fileId))
			continue
		}

		// Update the file with context information
		details := []domain.Detail{
			{
				Key:   bundle.RelationKeyCreatedInContext,
				Value: domain.String(contextInfo.contextId),
			},
		}

		if contextInfo.blockId != "" {
			details = append(details, domain.Detail{
				Key:   bundle.RelationKeyCreatedInBlockId,
				Value: domain.String(contextInfo.blockId),
			})
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		// Use detailsService to update the file
		if err := s.detailsService.SetDetails(nil, fileId, details); err != nil {
			l.Error("failed to update file context",
				zap.String("fileId", fileId),
				zap.Error(err))
			continue
		}

		migratedCount++
		l.Debug("migrated file context",
			zap.String("fileId", fileId),
			zap.String("contextId", contextInfo.contextId),
			zap.String("blockId", contextInfo.blockId))
	}

	l.Info("completed file context migration",
		zap.String("spaceId", spaceId),
		zap.Int("migratedCount", migratedCount),
		zap.Int("totalFiles", len(fileRecords)))

	// Mark migration as done with current version
	if err := s.markFileContextMigrationDone(workspaceId); err != nil {
		log.Warn("failed to mark migration done", zap.String("spaceId", spaceId), zap.Error(err))
	}

	return nil
}

// contextInfo stores the resolved context for a file
type contextInfo struct {
	contextId string
	blockId   string
}

// findCreationContext finds the creation context for a file by looking at incoming links
func (s *service) findCreationContext(fileObjectTs int64, fileId string, inboundLinks []spaceindex.IncomingLink) *contextInfo {
	if len(inboundLinks) == 0 {
		return nil
	}

	// Filter out system relations and self-references
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

	if len(links) == 0 {
		return nil
	}

	// Sort: block links first (by blockId for chronological order), then relation links
	sort.Slice(links, func(i, j int) bool {
		if links[i].BlockID != "" && links[j].BlockID == "" {
			return true
		}
		if links[i].BlockID == "" && links[j].BlockID != "" {
			return false
		}
		if links[i].BlockID != "" && links[j].BlockID != "" {
			return links[i].BlockID < links[j].BlockID
		}
		// Both are relation links, sort by relation key for determinism
		return links[i].RelationKey < links[j].RelationKey
	})

	// Prefer block links over relation links
	for _, link := range links {
		if link.BlockID != "" {
			blockTs, ok := time2.BsonIdToTimestamp(link.BlockID)
			if !ok {
				continue
			}
			if blockTs > fileObjectTs {
				// Block created after file object, skip
				continue
			}
			return &contextInfo{
				contextId: link.SourceID,
				blockId:   link.BlockID,
			}
		}
	}

	// Fall back to first relation link
	if len(links) > 0 {
		return &contextInfo{
			contextId: links[0].SourceID,
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

func (s *service) markFileContextMigrationDone(workspaceId string) error {
	return s.detailsService.SetDetails(nil, workspaceId, []domain.Detail{
		{Key: bundle.RelationKeyMigrationObjectContext, Value: domain.Int64(currentObjectContextMigrationVersion)},
	})
}
