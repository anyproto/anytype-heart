package objectgc

import (
	"context"
	"fmt"
	"slices"

	"github.com/anyproto/any-sync/app"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("objectgc")

const CName = "core.block.objectgc"

type ObjectGC interface {
	app.ComponentRunnable
	CheckObjectsOnLinksRemoval(sctx session.Context, spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) error
	CheckObjectsOnObjectArchived(sctx session.Context, spaceId, objectId string, isArchived bool) error
	CheckObjectsOnLinksRestored(sctx session.Context, spaceId, contextId string, addedLinks []string) error
}

// ObjectDeleter is an interface to delete objects by their full ID
type ObjectDeleter interface {
	DeleteObjectByFullID(id domain.FullID) error
}

// ObjectArchiver is an interface to archive objects
type ObjectArchiver interface {
	SetListIsArchived(sctx session.Context, ctx context.Context, objectIds []string, isArchived bool) error
}

// ParticipantProvider provides the current user's participant ID for a given space
type ParticipantProvider interface {
	MyParticipantId(spaceId string) string
}

type objectGC struct {
	objectDeleter       ObjectDeleter
	objectStore         objectstore.ObjectStore
	objectArchiver      ObjectArchiver
	backlinksWatcher    BacklinksFlusher
	participantProvider ParticipantProvider

	componentCtx context.Context
}

func New() ObjectGC {
	return &objectGC{}
}

type BacklinksFlusher interface {
	FlushUpdates()
}

func (gc *objectGC) Init(a *app.App) error {
	gc.objectDeleter = app.MustComponent[ObjectDeleter](a)
	gc.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	gc.objectArchiver = app.MustComponent[ObjectArchiver](a)
	gc.backlinksWatcher = app.MustComponent[BacklinksFlusher](a)
	gc.participantProvider = app.MustComponent[ParticipantProvider](a)
	return nil
}

func (gc *objectGC) Name() string {
	return CName
}

func (gc *objectGC) Run(ctx context.Context) error {
	gc.componentCtx = ctx
	return nil
}

func (gc *objectGC) Close(ctx context.Context) error {
	return nil
}

// CheckObjectsOnLinksRemoval checks if any of the removed links are objects that should be garbage collected.
// If onlyBlockIds is provided, it will only process objects created in those specific block IDs.
func (gc *objectGC) CheckObjectsOnLinksRemoval(sctx session.Context, spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) error {
	if len(removedLinks) == 0 {
		return nil
	}

	log.Debugf("checking %d removed links from context %s", len(removedLinks), contextId)

	// make sure we have all backlinks updates flushed to the store
	gc.backlinksWatcher.FlushUpdates()
	idx := gc.objectStore.SpaceIndex(spaceId)

	gcLayouts := makeGCEligibleLayouts()

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
			Value:       domain.Int64List(gcLayouts),
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

	// Query objects from removed links
	records, err := idx.Query(database.Query{
		Filters: filters,
	})
	if err != nil {
		return fmt.Errorf("query objects: %w", err)
	}

	var toArchive []string
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)

		// Filter out the current context and self-references from backlinks.
		backlinks := record.Details.GetStringList(bundle.RelationKeyBacklinks)
		activeBacklinks := lo.Filter(backlinks, func(link string, _ int) bool {
			return link != contextId && link != id
		})

		if len(activeBacklinks) > 0 {
			log.With("id", id).With("links", len(activeBacklinks)).Debugf("object has active backlinks, keeping")
			continue
		}

		// Object has no active backlinks and was created in this context - can be deleted or archived.
		shouldSkipBin := skipBin
		// Per-layout override: non-objects must never be permanently deleted.
		layout := model.ObjectTypeLayout(int32(record.Details.GetInt64(bundle.RelationKeyResolvedLayout)))
		if !slices.Contains(domain.FileLayouts, layout) {
			shouldSkipBin = false
		}
		if shouldSkipBin {
			// Additional safety: only permanently delete if the object was created by the current user.
			fileCreator := record.Details.GetString(bundle.RelationKeyCreator)
			myParticipantId := gc.participantProvider.MyParticipantId(spaceId)
			if fileCreator != myParticipantId {
				log.With("id", id).Debugf("object was created by another user - archiving instead of deleting")
				shouldSkipBin = false
			}
		}

		if shouldSkipBin {
			log.With("id", id).Debugf("deleting orphaned object created in context %s", contextId)
			if err := gc.deleteObject(spaceId, id); err != nil {
				log.With("id", id).Errorf("failed to delete object object: %v", err)
			}
		} else {
			log.With("id", id).Debugf("archiving orphaned object created in context %s", contextId)
			toArchive = append(toArchive, id)
		}
	}
	if err := gc.objectArchiver.SetListIsArchived(sctx, gc.componentCtx, toArchive, true); err != nil {
		return err
	}
	accumulateAutoArchiveEvent(sctx, toArchive, contextId)
	return nil
}

func (gc *objectGC) deleteObject(spaceId, id string) error {
	return gc.objectDeleter.DeleteObjectByFullID(domain.FullID{
		SpaceID:  spaceId,
		ObjectID: id,
	})
}

// CheckObjectsOnObjectArchived finds objects that should be garbage collected when objectId is archived or unarchived.
//
// Archive direction (isArchived=true): two disjoint queries are run.
//   - Query 1 (parent case): objects whose CreatedInContext == objectId. Since objectId is being archived
//     right now, no safety gate is needed — we just check that all other backlinks are archived or deleted.
//   - Query 2 (backlinker case): objects where objectId appears in their backlinks but is NOT their parent.
//     A safety gate checks that the object's actual parent (CreatedInContext) is already archived or deleted
//     before proceeding, preventing over-eager GC when the parent is still active.
//
// In both queries, a single batch Id IN [...] query resolves the active-status of all relevant IDs at once,
// rather than querying each backlink individually. objectId itself is always excluded from the active-backlinks
// check because it is currently being processed and may not yet be reflected as archived in the store.
//
// Unarchive direction (isArchived=false): only Query 1 runs, restoring objects whose parent is being unarchived,
// provided they have no other backlinks besides the parent itself.
func (gc *objectGC) CheckObjectsOnObjectArchived(sctx session.Context, spaceId, objectId string, isArchived bool) error {
	log.Debugf("checking objects on object archived: %s isArchived=%v", objectId, isArchived)
	idx := gc.objectStore.SpaceIndex(spaceId)

	d, err := idx.GetDetails(objectId)
	if err != nil {
		return fmt.Errorf("get details of object: %w", err)
	}
	if !slices.Contains(domain.GCEligibleLayouts, model.ObjectTypeLayout(int32(d.GetInt64(bundle.RelationKeyResolvedLayout)))) {
		// system/unsupported objects can't have GC-tracked children
		return nil
	}

	// make sure we have all backlinks updates flushed to the store
	gc.backlinksWatcher.FlushUpdates()

	gcLayouts := makeGCEligibleLayouts()

	if !isArchived {
		return gc.restoreObjectsOnUnarchive(idx, objectId, gcLayouts)
	}
	return gc.archiveOrphanedObjects(sctx, spaceId, idx, objectId, gcLayouts)
}

// archiveOrphanedObjects runs both queries and archives objects that have no active backlinks.
// Active-status of all referenced objects is resolved via at most two batch Id IN [...] queries —
// one per query set — rather than per-item detail lookups.
func (gc *objectGC) archiveOrphanedObjects(sctx session.Context, spaceId string, idx spaceindex.Store, objectId string, gcLayouts []int64) error {
	var toArchive []string

	// Query 1: objects whose parent context is the object being archived.
	// No safety gate needed — objectId IS the parent and is being archived right now.
	parentRecords, err := idx.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(objectId),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(gcLayouts),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query parent objects: %w", err)
	}

	if len(parentRecords) > 0 {
		// Collect all unique backlink IDs to check across all parent objects.
		linksToCheck := make(map[string]struct{})
		for _, record := range parentRecords {
			id := record.Details.GetString(bundle.RelationKeyId)
			for _, link := range record.Details.GetStringList(bundle.RelationKeyBacklinks) {
				if link != id && link != objectId {
					linksToCheck[link] = struct{}{}
				}
			}
		}

		// Single batch query: which of these backlinks are still active (not archived/deleted)?
		// The implicit IsArchived/IsDeleted filter from Query() excludes archived and deleted objects.
		activeIds, err := gc.queryActiveIds(idx, linksToCheck)
		if err != nil {
			return fmt.Errorf("query active backlinks for parent objects: %w", err)
		}

		for _, record := range parentRecords {
			id := record.Details.GetString(bundle.RelationKeyId)
			if hasActiveBacklinks(record.Details, id, objectId, activeIds) {
				log.Debugf("object %s has active backlinks, keeping", id)
				continue
			}
			log.Debugf("archiving orphaned object %s (parent %s archived)", id, objectId)
			toArchive = append(toArchive, id)
		}
	}

	// Query 2: objects where objectId is a backlinker but NOT the parent.
	// Safety gate per object: the object's own parent must already be archived or deleted.
	backlinkRecords, err := idx.Query(database.Query{
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
				Value:       domain.Int64List(gcLayouts),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query backlinker objects: %w", err)
	}

	if len(backlinkRecords) > 0 {
		// Collect all unique IDs to check: parent IDs (for safety gate) + backlink IDs.
		idsToCheck := make(map[string]struct{})
		for _, record := range backlinkRecords {
			id := record.Details.GetString(bundle.RelationKeyId)
			if parentId := record.Details.GetString(bundle.RelationKeyCreatedInContext); parentId != "" {
				idsToCheck[parentId] = struct{}{}
			}
			for _, link := range record.Details.GetStringList(bundle.RelationKeyBacklinks) {
				if link != id && link != objectId {
					idsToCheck[link] = struct{}{}
				}
			}
		}

		// Single batch query: which of these IDs are still active?
		activeIds, err := gc.queryActiveIds(idx, idsToCheck)
		if err != nil {
			return fmt.Errorf("query active ids for backlinker objects: %w", err)
		}

		for _, record := range backlinkRecords {
			id := record.Details.GetString(bundle.RelationKeyId)
			parentId := record.Details.GetString(bundle.RelationKeyCreatedInContext)

			// Safety gate: only GC if the object's own parent is already in the bin or fully deleted.
			if _, parentActive := activeIds[parentId]; parentActive {
				log.Debugf("object %s parent %s is still active, keeping", id, parentId)
				continue
			}
			if hasActiveBacklinks(record.Details, id, objectId, activeIds) {
				log.Debugf("object %s has active backlinks, keeping", id)
				continue
			}
			log.Debugf("archiving orphaned object %s (backlinker %s archived, parent %s already in bin)", id, objectId, parentId)
			toArchive = append(toArchive, id)
		}
	}

	if err := gc.objectArchiver.SetListIsArchived(sctx, gc.componentCtx, toArchive, true); err != nil {
		return err
	}
	accumulateAutoArchiveEvent(sctx, toArchive, objectId)
	return nil
}

// restoreObjectsOnUnarchive restores objects whose parent is being unarchived, provided they have
// no other backlinks besides the parent itself.
func (gc *objectGC) restoreObjectsOnUnarchive(idx spaceindex.Store, objectId string, gcLayouts []int64) error {
	records, err := idx.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(objectId),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(gcLayouts),
			},
			{
				// Explicit IsArchived filter suppresses the implicit IsArchived != true default,
				// allowing archived objects to appear in results.
				RelationKey: bundle.RelationKeyIsArchived,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query archived objects for restore: %w", err)
	}

	if len(records) == 0 {
		return nil
	}

	log.Debugf("found %d archived objects created in context %s to consider restoring", len(records), objectId)

	var toRestore []string
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		backlinks := record.Details.GetStringList(bundle.RelationKeyBacklinks)
		otherBacklinks := lo.Filter(backlinks, func(link string, _ int) bool {
			return link != objectId && link != id
		})
		if len(otherBacklinks) > 0 {
			log.Debugf("object %s has %d other backlinks, keeping archived", id, len(otherBacklinks))
			continue
		}
		toRestore = append(toRestore, id)
	}
	return gc.objectArchiver.SetListIsArchived(nil, gc.componentCtx, toRestore, false)
}

// queryActiveIds returns the subset of the given IDs that are active (not archived, not deleted).
// It issues a single Id IN [...] query and relies on the implicit IsArchived/IsDeleted filter from
// database.Query to exclude non-active objects.
func (gc *objectGC) queryActiveIds(idx spaceindex.Store, ids map[string]struct{}) (map[string]struct{}, error) {
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

// hasActiveBacklinks reports whether any backlink of the object (excluding id and currentlyArchivingId)
// appears in activeIds, meaning it is still an active (non-archived, non-deleted) object.
func hasActiveBacklinks(details *domain.Details, id, currentlyArchivingId string, activeIds map[string]struct{}) bool {
	for _, link := range details.GetStringList(bundle.RelationKeyBacklinks) {
		if link == id || link == currentlyArchivingId {
			continue
		}
		if _, active := activeIds[link]; active {
			return true
		}
	}
	return false
}

// CheckObjectsOnLinksRestored unarchives objects that were previously GC'd when links are re-added
// to a context object (e.g. via undo). For each added link that is an archived object whose
// CreatedInContext matches the context, the object is restored unconditionally — the active link
// in the page is sufficient justification to unarchive it.
func (gc *objectGC) CheckObjectsOnLinksRestored(sctx session.Context, spaceId, contextId string, addedLinks []string) error {
	if len(addedLinks) == 0 {
		return nil
	}

	log.Debugf("checking %d restored links in context %s", len(addedLinks), contextId)

	gc.backlinksWatcher.FlushUpdates()
	idx := gc.objectStore.SpaceIndex(spaceId)

	gcLayouts := makeGCEligibleLayouts()

	// Find archived objects among the added links that belong to this context.
	// Explicit IsArchived == true suppresses the implicit IsArchived != true default filter.
	records, err := idx.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyId,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(addedLinks),
			},
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(contextId),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(gcLayouts),
			},
			{
				RelationKey: bundle.RelationKeyIsArchived,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query archived objects for restore: %w", err)
	}

	var toRestore []string
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		log.Debugf("restoring archived object %s after link re-added to context %s", id, contextId)
		toRestore = append(toRestore, id)
	}
	return gc.objectArchiver.SetListIsArchived(nil, gc.componentCtx, toRestore, false)
}

func makeGCEligibleLayouts() []int64 {
	layouts := make([]int64, 0, len(domain.GCEligibleLayouts))
	for _, layout := range domain.GCEligibleLayouts {
		layouts = append(layouts, int64(layout))
	}
	return layouts
}

// accumulateAutoArchiveEvent merges objectIds into the auto-archive event already present in sctx,
// or appends a new one if none exists. All callers sharing the same sctx will end up with exactly
// one EventObjectAutoArchive message containing the union of all archived IDs, which is what the
// RPC layer harvests via GetResponseEvent at the end of each call.
func accumulateAutoArchiveEvent(sctx session.Context, objectIds []string, smartBlockId string) {
	if sctx == nil || len(objectIds) == 0 {
		return
	}
	msgs := sctx.GetMessages()
	for i, msg := range msgs {
		if existing, ok := msg.Value.(*pb.EventMessageValueOfObjectAutoArchive); ok {
			seen := make(map[string]struct{}, len(existing.ObjectAutoArchive.ObjectIds)+len(objectIds))
			merged := make([]string, 0, len(existing.ObjectAutoArchive.ObjectIds)+len(objectIds))
			for _, id := range existing.ObjectAutoArchive.ObjectIds {
				if _, dup := seen[id]; !dup {
					seen[id] = struct{}{}
					merged = append(merged, id)
				}
			}
			for _, id := range objectIds {
				if _, dup := seen[id]; !dup {
					seen[id] = struct{}{}
					merged = append(merged, id)
				}
			}
			msgs[i] = &pb.EventMessage{
				Value: &pb.EventMessageValueOfObjectAutoArchive{
					ObjectAutoArchive: &pb.EventObjectAutoArchive{ObjectIds: merged},
				},
			}
			sctx.SetMessages(smartBlockId, msgs)
			return
		}
	}
	msgs = append(msgs, &pb.EventMessage{
		Value: &pb.EventMessageValueOfObjectAutoArchive{
			ObjectAutoArchive: &pb.EventObjectAutoArchive{ObjectIds: objectIds},
		},
	})
	sctx.SetMessages(smartBlockId, msgs)
}
