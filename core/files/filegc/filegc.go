package filegc

import (
	"context"
	"fmt"

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

const CName = "filegc"

type FileGC interface {
	app.ComponentRunnable
	CheckFilesOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds ...string) error
}

// ObjectDeleter is an interface to delete objects by their full ID
type ObjectDeleter interface {
	DeleteObjectByFullID(id domain.FullID) error
}

// ObjectArchiver is an interface to archive objects
type ObjectArchiver interface {
	SetIsArchived(objectId string, isArchived bool) error
}

type fileGC struct {
	objectDeleter  ObjectDeleter
	objectStore    objectstore.ObjectStore
	objectArchiver ObjectArchiver
}

func New() FileGC {
	return &fileGC{}
}

func (gc *fileGC) Init(a *app.App) error {
	gc.objectDeleter = app.MustComponent[ObjectDeleter](a)
	gc.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	gc.objectArchiver = app.MustComponent[ObjectArchiver](a)
	return nil
}

func (gc *fileGC) Name() string {
	return CName
}

func (gc *fileGC) Run(ctx context.Context) error {
	return nil
}

func (gc *fileGC) Close(ctx context.Context) error {
	return nil
}

// CheckFilesOnLinksRemoval checks if any of the removed links are file objects that should be garbage collected
// If onlyBlockIds is provided, it will only process files created in those specific block IDs
func (gc *fileGC) CheckFilesOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds ...string) error {
	if len(removedLinks) == 0 {
		return nil
	}

	log.Warnf("checking %d removed links from context %s", len(removedLinks), contextId)

	// Get space index
	spaceIndex := gc.objectStore.SpaceIndex(spaceId)
	if spaceIndex == nil {
		return fmt.Errorf("space index not found for space %s", spaceId)
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
			Value:       domain.Int64List([]int64{int64(model.ObjectType_file), int64(model.ObjectType_image)}),
		},
	}

	// If onlyBlockIds is provided, add filter for CreatedInBlockId
	if len(onlyBlockIds) > 0 {
		filters = append(filters, database.FilterRequest{
			RelationKey: bundle.RelationKeyCreatedInBlockId,
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

	for _, record := range fileRecords {
		fileId := record.Details.GetString(bundle.RelationKeyId)

		// Check if file has any backlinks (references from other objects)
		backlinks := record.Details.GetStringList(bundle.RelationKeyBacklinks)

		// Filter out the current context from backlinks
		activeBacklinks := lo.Filter(backlinks, func(link string, _ int) bool {
			return link != contextId
		})

		if len(activeBacklinks) > 0 {
			log.Debugf("file %s has %d active backlinks, keeping", fileId, len(activeBacklinks))
			continue
		}

		// File has no active backlinks and was created in this context - can be deleted or archived
		if skipBin {
			log.Debugf("deleting orphaned file %s created in context %s", fileId, contextId)
			// Delete the file object
			if err := gc.deleteFileObject(spaceId, fileId); err != nil {
				log.Errorf("failed to delete file object %s: %v", fileId, err)
				// Continue with other files even if one fails
			}
		} else {
			log.Debugf("archiving orphaned file %s created in context %s", fileId, contextId)
			// Archive the file object
			if err := gc.objectArchiver.SetIsArchived(fileId, true); err != nil {
				log.Errorf("failed to archive file object %s: %v", fileId, err)
				// Continue with other files even if one fails
			}
		}
	}

	return nil
}

func (gc *fileGC) deleteFileObject(spaceId, fileId string) error {
	// Delete the file object using the full ID
	return gc.objectDeleter.DeleteObjectByFullID(domain.FullID{
		SpaceID:  spaceId,
		ObjectID: fileId,
	})
}
