package migration

import (
	"context"
	"fmt"
	"slices"
	"sort"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "files.contextmigration"

var log = logging.Logger(CName)

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

3. Build incomingLinksMap: targetId -> []incomingLinkInfo
   - Iterate all objects in space
   - For each object, get its outbound links (GetOutboundLinksDetailedById)
   - Index these links by TARGET, creating an inverse lookup
   - Result: incomingLinksMap[fileId] returns all objects linking TO that file
   - Skip system relations (Creator, Type, Backlinks, etc.) - not meaningful for context

4. For each file, find creation context:
   - Look up incomingLinksMap[fileId] to get all objects referencing this file
   - Priority: block links (have blockId) over relation links (no blockId)
   - Sort by blockId and pick first - blockId is BSON ObjectId containing timestamp,
     so lexicographic sort gives chronological order (oldest block first = original context)
   - Set CreatedInContext = source objectId, CreatedInBlockId = blockId (if block link)
*/

// ContextMigrationService migrates existing files to have creation context fields
type ContextMigrationService interface {
	app.Component
	// MigrateSpace migrates all files in a specific space
	MigrateSpace(ctx context.Context, store spaceindex.Store) error
}

type contextMigrationService struct {
	objectStore    objectstore.ObjectStore
	detailsService detailservice.Service
}

func NewContextMigrationService() ContextMigrationService {
	return &contextMigrationService{}
}

func (s *contextMigrationService) Name() string {
	return CName
}

func (s *contextMigrationService) Init(a *app.App) error {
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.detailsService = app.MustComponent[detailservice.Service](a)
	log.Info("started")
	return nil
}

func (s *contextMigrationService) MigrateSpace(ctx context.Context, spaceIndex spaceindex.Store) error {
	spaceId := spaceIndex.SpaceId()
	log.Info("starting file context migration", zap.String("spaceId", spaceId))

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

	log.Info("found files without context",
		zap.String("spaceId", spaceId),
		zap.Int("count", len(fileRecords)))

	if len(fileRecords) == 0 {
		return nil
	}

	// Step 2: Build a map of incoming links (indexed by target) for all objects in the space
	incomingLinksMap := s.buildIncomingLinksMap(spaceId, spaceIndex)

	// Step 3: Process each file
	migratedCount := 0
	for _, fileRecord := range fileRecords {
		fileId := fileRecord.Details.GetString(bundle.RelationKeyId)

		// Find the creation context for this file
		contextInfo := s.findCreationContext(fileId, incomingLinksMap)
		if contextInfo == nil {
			log.Debug("no creation context found for file", zap.String("fileId", fileId))
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
			log.Error("failed to update file context",
				zap.String("fileId", fileId),
				zap.Error(err))
			continue
		}

		migratedCount++
		log.Debug("migrated file context",
			zap.String("fileId", fileId),
			zap.String("contextId", contextInfo.contextId),
			zap.String("blockId", contextInfo.blockId))
	}

	log.Info("completed file context migration",
		zap.String("spaceId", spaceId),
		zap.Int("migratedCount", migratedCount),
		zap.Int("totalFiles", len(fileRecords)))

	return nil
}

// incomingLinkInfo stores information about an incoming link to a target (from the target's perspective)
type incomingLinkInfo struct {
	// source object that links to the target
	objectId    string
	blockId     string // blockID or chat message ID of the source object
	relationKey string
}

// contextInfo stores the resolved context for a file
type contextInfo struct {
	contextId string
	blockId   string
}

// buildIncomingLinksMap builds a map of targetId -> []incomingLinkInfo for all objects in the space
// The map is indexed by target, so looking up a file ID returns all objects that link TO that file
func (s *contextMigrationService) buildIncomingLinksMap(spaceId string, spaceIndex spaceindex.Store) map[string][]incomingLinkInfo {
	incomingLinksMap := make(map[string][]incomingLinkInfo)

	// Query all objects in the space
	allRecords, err := spaceIndex.Query(database.Query{})
	if err != nil {
		log.Error("failed to query all objects", zap.Error(err))
		return incomingLinksMap
	}

	for _, record := range allRecords {
		sourceId := record.Details.GetString(bundle.RelationKeyId)
		if sourceId == "" {
			continue
		}

		// Get outbound links from this source, then index them by target (creating incoming links index)
		detailedLinks, err := spaceIndex.GetOutboundLinksDetailedById(sourceId)
		if err == nil && len(detailedLinks) > 0 {
			for _, link := range detailedLinks {
				if link.TargetID == sourceId {
					continue
				}
				if slices.Contains(systemRelationsToSkip, domain.RelationKey(link.RelationKey)) {
					continue
				}
				info := incomingLinkInfo{
					objectId:    sourceId,
					blockId:     link.BlockID,
					relationKey: link.RelationKey,
				}
				incomingLinksMap[link.TargetID] = append(incomingLinksMap[link.TargetID], info)
			}
		}
	}

	return incomingLinksMap
}

// findCreationContext finds the creation context for a file by looking at incoming links
func (s *contextMigrationService) findCreationContext(fileId string, incomingLinksMap map[string][]incomingLinkInfo) *contextInfo {
	links, ok := incomingLinksMap[fileId]
	if !ok || len(links) == 0 {
		return nil
	}

	sort.Slice(links, func(i, j int) bool {
		if links[i].blockId == "" && links[j].blockId == "" {
			// no meaning here, just to be deterministic
			return links[i].relationKey < links[j].relationKey
		}
		return links[i].blockId < links[j].blockId
	})

	// Prefer block links over relation links
	for _, link := range links {
		if link.blockId != "" {
			return &contextInfo{
				contextId: link.objectId,
				blockId:   link.blockId,
			}
		}
	}

	// Fall back to relation links
	if len(links) > 0 {
		return &contextInfo{
			contextId: links[0].objectId,
		}
	}

	return nil
}
