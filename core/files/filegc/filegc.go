package filegc

import (
	"context"
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("filegc")

const CName = "core.files.filegc"

type FileGC interface {
	app.ComponentRunnable
	CheckFilesOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) error
	CheckFilesOnContextArchived(spaceId, contextId string, isArchived bool) error
}

// ObjectDeleter is an interface to delete objects by their full ID
type ObjectDeleter interface {
	DeleteObjectByFullID(id domain.FullID) error
}

// ObjectArchiver is an interface to archive objects
type ObjectArchiver interface {
	SetListIsArchived(ctx context.Context, objectIds []string, isArchived bool) error
}

// ParticipantProvider provides the current user's participant ID for a given space
type ParticipantProvider interface {
	MyParticipantId(spaceId string) string
}

type fileGC struct {
	objectDeleter       ObjectDeleter
	objectStore         objectstore.ObjectStore
	objectArchiver      ObjectArchiver
	backlinksWatcher    BacklinksFlusher
	participantProvider ParticipantProvider

	componentCtx context.Context
}

func New() FileGC {
	return &fileGC{}
}

type BacklinksFlusher interface {
	FlushUpdates()
}

func (gc *fileGC) Init(a *app.App) error {
	gc.objectDeleter = app.MustComponent[ObjectDeleter](a)
	gc.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	gc.objectArchiver = app.MustComponent[ObjectArchiver](a)
	gc.backlinksWatcher = app.MustComponent[BacklinksFlusher](a)
	gc.participantProvider = app.MustComponent[ParticipantProvider](a)
	return nil
}

func (gc *fileGC) Name() string {
	return CName
}

func (gc *fileGC) Run(ctx context.Context) error {
	gc.componentCtx = ctx
	return nil
}

func (gc *fileGC) Close(ctx context.Context) error {
	return nil
}

// CheckFilesOnLinksRemoval checks if any of the removed links are file objects that should be garbage collected
// If onlyBlockIds is provided, it will only process files created in those specific block IDs
func (gc *fileGC) CheckFilesOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) error {
	if len(removedLinks) == 0 {
		return nil
	}

	log.Debugf("checking %d removed links from context %s", len(removedLinks), contextId)

	// make sure we have all backlinks updates flushed to the store
	gc.backlinksWatcher.FlushUpdates()
	// Get space index
	spaceIndex := gc.objectStore.SpaceIndex(spaceId)

	fileLayouts := make([]int64, 0, len(domain.FileLayouts))
	for _, layout := range domain.FileLayouts {
		fileLayouts = append(fileLayouts, int64(layout))
	}

	// Build query filters
	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyId,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList(removedLinks),
		},
		{
			RelationKey: bundle.RelationKeyCreatedInContext,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String(contextId),
		},
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List(fileLayouts),
		},
	}

	// If onlyBlockIds is provided, add filter for CreatedInContextRef
	if len(onlyBlockIds) > 0 {
		filters = append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyCreatedInContextRef,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList(onlyBlockIds),
		})
	}

	// Query file objects from removed links
	fileRecords, err := spaceIndex.Query(database.Query{
		Filters: filters,
	})
	if err != nil {
		return fmt.Errorf("failed to query file objects: %w", err)
	}

	var toArchive []string
	for _, record := range fileRecords {
		fileId := record.Details.GetString(bundle.RelationKeyId)

		// Check if file has any backlinks (references from other objects)
		backlinks := record.Details.GetStringList(bundle.RelationKeyBacklinks)

		// Filter out the current context from backlinks
		// Also filter out self-references (link != fileId) to prevent files from keeping themselves alive
		activeBacklinks := lo.Filter(backlinks, func(link string, _ int) bool {
			return link != contextId && link != fileId
		})

		if len(activeBacklinks) > 0 {
			log.With("fileId", fileId).With("links", len(activeBacklinks)).Debugf("file has active backlinks, keeping")
			continue
		}

		// File has no active backlinks and was created in this context - can be deleted or archived
		shouldSkipBin := skipBin
		if shouldSkipBin {
			// Additional safety: only permanently delete if the file was created by the current user
			fileCreator := record.Details.GetString(bundle.RelationKeyCreator)
			myParticipantId := gc.participantProvider.MyParticipantId(spaceId)
			if fileCreator != myParticipantId {
				log.With("fileId", fileId).Debugf("file was created by another user - archiving instead of deleting")
				shouldSkipBin = false
			}
		}

		if shouldSkipBin {
			log.With("fileId", fileId).Debugf("deleting orphaned file created in context %s", contextId)
			// Delete the file object
			if err := gc.deleteFileObject(spaceId, fileId); err != nil {
				log.With("fileId", fileId).Errorf("failed to delete file object: %v", err)
				// Continue with other files even if one fails
			}
		} else {
			log.With("fileId", fileId).Debugf("archiving orphaned file created in context %s", contextId)
			toArchive = append(toArchive, fileId)
		}
	}
	return gc.objectArchiver.SetListIsArchived(gc.componentCtx, toArchive, true)
}

func (gc *fileGC) deleteFileObject(spaceId, fileId string) error {
	// Delete the file object using the full ID
	return gc.objectDeleter.DeleteObjectByFullID(domain.FullID{
		SpaceID:  spaceId,
		ObjectID: fileId,
	})
}

// CheckFilesOnContextArchived finds all files created in the given context and archives/unarchived them
// This should be called when the context object itself is being archived/unarchived
func (gc *fileGC) CheckFilesOnContextArchived(spaceId, contextId string, isArchived bool) error {
	log.Debugf("checking files on context deletion: %s", contextId)
	spaceIndex := gc.objectStore.SpaceIndex(spaceId)
	d, err := spaceIndex.GetDetails(contextId)
	if err != nil {
		return fmt.Errorf("failed to get details of context object: %w", err)
	}

	if slices.Contains(domain.FileLayouts, model.ObjectTypeLayout(int32(d.GetInt64(bundle.RelationKeyResolvedLayout)))) {
		// files can't have files as children, so there's no need to check them
		return nil
	}

	fileLayouts := make([]int64, 0, len(domain.FileLayouts))
	for _, layout := range domain.FileLayouts {
		fileLayouts = append(fileLayouts, int64(layout))
	}

	// make sure we have all backlinks updates flushed to the store
	gc.backlinksWatcher.FlushUpdates()

	// Query all files created in this context
	fileRecords, err := spaceIndex.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(contextId),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(fileLayouts),
			},
			{
				RelationKey: bundle.RelationKeyIsArchived,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(isArchived),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to query file objects: %w", err)
	}

	if len(fileRecords) == 0 {
		return nil
	}

	log.Debugf("found %d files created in context %s", len(fileRecords), contextId)

	var toProcess []string
	for _, record := range fileRecords {
		fileId := record.Details.GetString(bundle.RelationKeyId)

		// Check if file has any backlinks from other objects (not the context being deleted)
		backlinks := record.Details.GetStringList(bundle.RelationKeyBacklinks)
		activeBacklinks := lo.Filter(backlinks, func(link string, _ int) bool {
			return link != contextId && link != fileId
		})

		if len(activeBacklinks) > 0 {
			log.Debugf("file %s has %d active backlinks, keeping", fileId, len(activeBacklinks))
			continue
		}

		// File has no active backlinks - archive it since the context is being deleted
		toProcess = append(toProcess, fileId)
	}
	return gc.objectArchiver.SetListIsArchived(gc.componentCtx, toProcess, isArchived)
}
