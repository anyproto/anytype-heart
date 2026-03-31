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
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("filegc")

const CName = "core.files.filegc"

type FileGC interface {
	app.ComponentRunnable
	CheckFilesOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) error
	CheckFilesOnObjectArchived(spaceId, objectId string, isArchived bool) error
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

// CheckFilesOnLinksRemoval checks if any of the removed links are file objects that should be garbage collected.
// If onlyBlockIds is provided, it will only process files created in those specific block IDs.
func (gc *fileGC) CheckFilesOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) error {
	if len(removedLinks) == 0 {
		return nil
	}

	log.Debugf("checking %d removed links from context %s", len(removedLinks), contextId)

	// make sure we have all backlinks updates flushed to the store
	gc.backlinksWatcher.FlushUpdates()
	idx := gc.objectStore.SpaceIndex(spaceId)

	fileLayouts := makeFileLayouts()

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
	fileRecords, err := idx.Query(database.Query{
		Filters: filters,
	})
	if err != nil {
		return fmt.Errorf("query file objects: %w", err)
	}

	var toArchive []string
	for _, record := range fileRecords {
		fileId := record.Details.GetString(bundle.RelationKeyId)

		// Filter out the current context and self-references from backlinks.
		backlinks := record.Details.GetStringList(bundle.RelationKeyBacklinks)
		activeBacklinks := lo.Filter(backlinks, func(link string, _ int) bool {
			return link != contextId && link != fileId
		})

		if len(activeBacklinks) > 0 {
			log.With("fileId", fileId).With("links", len(activeBacklinks)).Debugf("file has active backlinks, keeping")
			continue
		}

		// File has no active backlinks and was created in this context - can be deleted or archived.
		shouldSkipBin := skipBin
		if shouldSkipBin {
			// Additional safety: only permanently delete if the file was created by the current user.
			fileCreator := record.Details.GetString(bundle.RelationKeyCreator)
			myParticipantId := gc.participantProvider.MyParticipantId(spaceId)
			if fileCreator != myParticipantId {
				log.With("fileId", fileId).Debugf("file was created by another user - archiving instead of deleting")
				shouldSkipBin = false
			}
		}

		if shouldSkipBin {
			log.With("fileId", fileId).Debugf("deleting orphaned file created in context %s", contextId)
			if err := gc.deleteFileObject(spaceId, fileId); err != nil {
				log.With("fileId", fileId).Errorf("failed to delete file object: %v", err)
			}
		} else {
			log.With("fileId", fileId).Debugf("archiving orphaned file created in context %s", contextId)
			toArchive = append(toArchive, fileId)
		}
	}
	return gc.objectArchiver.SetListIsArchived(gc.componentCtx, toArchive, true)
}

func (gc *fileGC) deleteFileObject(spaceId, fileId string) error {
	return gc.objectDeleter.DeleteObjectByFullID(domain.FullID{
		SpaceID:  spaceId,
		ObjectID: fileId,
	})
}

// CheckFilesOnObjectArchived finds file objects that should be garbage collected when objectId is archived or unarchived.
//
// Archive direction (isArchived=true): two disjoint queries are run.
//   - Query 1 (parent case): files whose CreatedInContext == objectId. Since objectId is being archived
//     right now, no safety gate is needed — we just check that all other backlinks are archived or deleted.
//   - Query 2 (backlinker case): files where objectId appears in their backlinks but is NOT their parent.
//     A safety gate checks that the file's actual parent (CreatedInContext) is already archived or deleted
//     before proceeding, preventing over-eager GC when the parent is still active.
//
// In both queries, a single batch Id IN [...] query resolves the active-status of all relevant IDs at once,
// rather than querying each backlink individually. objectId itself is always excluded from the active-backlinks
// check because it is currently being processed and may not yet be reflected as archived in the store.
//
// Unarchive direction (isArchived=false): only Query 1 runs, restoring files whose parent is being unarchived,
// provided they have no other backlinks besides the parent itself.
func (gc *fileGC) CheckFilesOnObjectArchived(spaceId, objectId string, isArchived bool) error {
	log.Debugf("checking files on object archived: %s isArchived=%v", objectId, isArchived)
	idx := gc.objectStore.SpaceIndex(spaceId)

	d, err := idx.GetDetails(objectId)
	if err != nil {
		return fmt.Errorf("get details of object: %w", err)
	}
	if slices.Contains(domain.FileLayouts, model.ObjectTypeLayout(int32(d.GetInt64(bundle.RelationKeyResolvedLayout)))) {
		// files can't have files as children, so there's no need to check them
		return nil
	}

	// make sure we have all backlinks updates flushed to the store
	gc.backlinksWatcher.FlushUpdates()

	fileLayouts := makeFileLayouts()

	if !isArchived {
		return gc.restoreFilesOnUnarchive(idx, objectId, fileLayouts)
	}
	return gc.archiveOrphanedFiles(idx, objectId, fileLayouts)
}

// archiveOrphanedFiles runs both queries and archives files that have no active backlinks.
// Active-status of all referenced objects is resolved via at most two batch Id IN [...] queries —
// one per query set — rather than per-item detail lookups.
func (gc *fileGC) archiveOrphanedFiles(idx spaceindex.Store, objectId string, fileLayouts []int64) error {
	var toArchive []string

	// Query 1: files whose parent context is the object being archived.
	// No safety gate needed — objectId IS the parent and is being archived right now.
	parentFiles, err := idx.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(objectId),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(fileLayouts),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query parent files: %w", err)
	}

	if len(parentFiles) > 0 {
		// Collect all unique backlink IDs to check across all parent files.
		linksToCheck := make(map[string]struct{})
		for _, record := range parentFiles {
			fileId := record.Details.GetString(bundle.RelationKeyId)
			for _, link := range record.Details.GetStringList(bundle.RelationKeyBacklinks) {
				if link != fileId && link != objectId {
					linksToCheck[link] = struct{}{}
				}
			}
		}

		// Single batch query: which of these backlinks are still active (not archived/deleted)?
		// The implicit IsArchived/IsDeleted filter from Query() excludes archived and deleted objects.
		activeIds, err := gc.queryActiveIds(idx, linksToCheck)
		if err != nil {
			return fmt.Errorf("query active backlinks for parent files: %w", err)
		}

		for _, record := range parentFiles {
			fileId := record.Details.GetString(bundle.RelationKeyId)
			if hasActiveBacklinks(record.Details, fileId, objectId, activeIds) {
				log.Debugf("file %s has active backlinks, keeping", fileId)
				continue
			}
			log.Debugf("archiving orphaned file %s (parent %s archived)", fileId, objectId)
			toArchive = append(toArchive, fileId)
		}
	}

	// Query 2: files where objectId is a backlinker but NOT the parent.
	// Safety gate per file: the file's own parent must already be archived or deleted.
	backlinkFiles, err := idx.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyBacklinks,
				Condition:   model.BlockContentDataviewFilter_AllIn,
				Value:       domain.StringList([]string{objectId}),
			},
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.String(objectId),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(fileLayouts),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query backlinker files: %w", err)
	}

	if len(backlinkFiles) > 0 {
		// Collect all unique IDs to check: parent IDs (for safety gate) + backlink IDs.
		idsToCheck := make(map[string]struct{})
		for _, record := range backlinkFiles {
			fileId := record.Details.GetString(bundle.RelationKeyId)
			if parentId := record.Details.GetString(bundle.RelationKeyCreatedInContext); parentId != "" {
				idsToCheck[parentId] = struct{}{}
			}
			for _, link := range record.Details.GetStringList(bundle.RelationKeyBacklinks) {
				if link != fileId && link != objectId {
					idsToCheck[link] = struct{}{}
				}
			}
		}

		// Single batch query: which of these IDs are still active?
		activeIds, err := gc.queryActiveIds(idx, idsToCheck)
		if err != nil {
			return fmt.Errorf("query active ids for backlinker files: %w", err)
		}

		for _, record := range backlinkFiles {
			fileId := record.Details.GetString(bundle.RelationKeyId)
			parentId := record.Details.GetString(bundle.RelationKeyCreatedInContext)

			// Safety gate: only GC if the file's own parent is already in the bin or fully deleted.
			if _, parentActive := activeIds[parentId]; parentActive {
				log.Debugf("file %s parent %s is still active, keeping", fileId, parentId)
				continue
			}
			if hasActiveBacklinks(record.Details, fileId, objectId, activeIds) {
				log.Debugf("file %s has active backlinks, keeping", fileId)
				continue
			}
			log.Debugf("archiving orphaned file %s (backlinker %s archived, parent %s already in bin)", fileId, objectId, parentId)
			toArchive = append(toArchive, fileId)
		}
	}

	return gc.objectArchiver.SetListIsArchived(gc.componentCtx, toArchive, true)
}

// restoreFilesOnUnarchive restores files whose parent is being unarchived, provided they have
// no other backlinks besides the parent itself.
func (gc *fileGC) restoreFilesOnUnarchive(idx spaceindex.Store, objectId string, fileLayouts []int64) error {
	fileRecords, err := idx.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(objectId),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(fileLayouts),
			},
			{
				// Explicit IsArchived filter suppresses the implicit IsArchived != true default,
				// allowing archived files to appear in results.
				RelationKey: bundle.RelationKeyIsArchived,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query archived files for restore: %w", err)
	}

	if len(fileRecords) == 0 {
		return nil
	}

	log.Debugf("found %d archived files created in context %s to consider restoring", len(fileRecords), objectId)

	var toRestore []string
	for _, record := range fileRecords {
		fileId := record.Details.GetString(bundle.RelationKeyId)
		backlinks := record.Details.GetStringList(bundle.RelationKeyBacklinks)
		otherBacklinks := lo.Filter(backlinks, func(link string, _ int) bool {
			return link != objectId && link != fileId
		})
		if len(otherBacklinks) > 0 {
			log.Debugf("file %s has %d other backlinks, keeping archived", fileId, len(otherBacklinks))
			continue
		}
		toRestore = append(toRestore, fileId)
	}
	return gc.objectArchiver.SetListIsArchived(gc.componentCtx, toRestore, false)
}

// queryActiveIds returns the subset of the given IDs that are active (not archived, not deleted).
// It issues a single Id IN [...] query and relies on the implicit IsArchived/IsDeleted filter from
// database.Query to exclude non-active objects.
func (gc *fileGC) queryActiveIds(idx spaceindex.Store, ids map[string]struct{}) (map[string]struct{}, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	idList := make([]string, 0, len(ids))
	for id := range ids {
		idList = append(idList, id)
	}
	records, err := idx.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyId,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(idList),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query active ids: %w", err)
	}
	active := make(map[string]struct{}, len(records))
	for _, r := range records {
		active[r.Details.GetString(bundle.RelationKeyId)] = struct{}{}
	}
	return active, nil
}

// hasActiveBacklinks reports whether any backlink of the file (excluding fileId and currentlyArchivingId)
// appears in activeIds, meaning it is still an active (non-archived, non-deleted) object.
func hasActiveBacklinks(details *domain.Details, fileId, currentlyArchivingId string, activeIds map[string]struct{}) bool {
	for _, link := range details.GetStringList(bundle.RelationKeyBacklinks) {
		if link == fileId || link == currentlyArchivingId {
			continue
		}
		if _, active := activeIds[link]; active {
			return true
		}
	}
	return false
}

func makeFileLayouts() []int64 {
	layouts := make([]int64, 0, len(domain.FileLayouts))
	for _, layout := range domain.FileLayouts {
		layouts = append(layouts, int64(layout))
	}
	return layouts
}
