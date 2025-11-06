package migration

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"sync"

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
	"github.com/anyproto/anytype-heart/space"
)

const CName = "files.contextmigration"

var log = logging.Logger(CName)

// ContextMigrationService migrates existing files to have creation context fields
type ContextMigrationService interface {
	app.Component
	// MigrateSpace migrates all files in a specific space
	MigrateSpace(ctx context.Context, store spaceindex.Store) error
	// MigrateAllSpaces migrates files in all spaces
	MigrateAllSpaces(ctx context.Context) error
}

type contextMigrationService struct {
	objectStore    objectstore.ObjectStore
	detailsService detailservice.Service
	spaceService   space.Service

	mu sync.Mutex
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
	s.spaceService = app.MustComponent[space.Service](a)
	log.Info("started")
	return nil
}

func (s *contextMigrationService) MigrateAllSpaces(ctx context.Context) error {
	log.Info("starting file context migration for all spaces")

	var spaceCount int
	err := s.objectStore.IterateSpaceIndex(func(spaceIndex spaceindex.Store) error {
		spaceCount++
		spaceId := spaceIndex.SpaceId()
		if err := s.MigrateSpace(ctx, spaceIndex); err != nil {
			log.Error("failed to migrate space", zap.String("spaceId", spaceId), zap.Error(err))
			// Continue with other spaces - don't return error
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to iterate space indexes: %w", err)
	}

	log.Info("completed file context migration for all spaces", zap.Int("spaceCount", spaceCount))
	return nil
}

func (s *contextMigrationService) MigrateSpace(ctx context.Context, spaceIndex spaceindex.Store) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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

	// Step 2: Build a map of all outgoing links in the space
	outgoingLinksMap := s.buildOutgoingLinksMap(spaceId, spaceIndex)

	// Step 3: Process each file
	migratedCount := 0
	for _, fileRecord := range fileRecords {
		fileId := fileRecord.Details.GetString(bundle.RelationKeyId)

		// Find the creation context for this file
		contextInfo := s.findCreationContext(fileId, outgoingLinksMap)
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

// OutgoingLinkInfo stores information about an outgoing link
type outgoingLinkInfo struct {
	// source
	objectId    string
	blockId     string // blockID or chat message ID of the source object
	relationKey string

	targetId string // target object ID
}

// contextInfo stores the resolved context for a file
type contextInfo struct {
	contextId string
	blockId   string
}

// buildOutgoingLinksMap builds a map of targetId -> []outgoingLinkInfo for all objects in the space
func (s *contextMigrationService) buildOutgoingLinksMap(spaceId string, spaceIndex spaceindex.Store) map[string][]outgoingLinkInfo {
	outgoingLinksMap := make(map[string][]outgoingLinkInfo)

	// Query all objects in the space
	allRecords, err := spaceIndex.Query(database.Query{})
	if err != nil {
		log.Error("failed to query all objects", zap.Error(err))
		return outgoingLinksMap
	}

	for _, record := range allRecords {
		sourceId := record.Details.GetString(bundle.RelationKeyId)
		if sourceId == "" {
			continue
		}

		// Try to get detailed links first
		detailedLinks, err := spaceIndex.GetOutboundLinksDetailedById(sourceId)
		if err == nil && len(detailedLinks) > 0 {
			for _, link := range detailedLinks {
				if link.TargetID == sourceId {
					continue
				}
				if slices.Contains([]domain.RelationKey{
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
				}, domain.RelationKey(link.RelationKey)) {
					continue
				}
				fmt.Printf("Processing detailed link: %s -> %s (block: %s, relation: %s)\n", sourceId, link.TargetID, link.BlockID, link.RelationKey)
				info := outgoingLinkInfo{
					objectId:    sourceId,
					targetId:    link.TargetID,
					blockId:     link.BlockID,
					relationKey: link.RelationKey,
				}
				outgoingLinksMap[link.TargetID] = append(outgoingLinksMap[link.TargetID], info)
			}
		}
	}

	return outgoingLinksMap
}

// findCreationContext finds the creation context for a file by looking at incoming links
func (s *contextMigrationService) findCreationContext(fileId string, outgoingLinksMap map[string][]outgoingLinkInfo) *contextInfo {
	links, ok := outgoingLinksMap[fileId]
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
