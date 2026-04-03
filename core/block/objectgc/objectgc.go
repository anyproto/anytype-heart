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
	// CheckObjectsOnObjectArchived finds objects that should be archived or restored when objectId changes state.
	// the caller is responsible for actual archive-object operation
	CheckObjectsOnObjectArchived(spaceId, objectId string, isArchived bool) ([]string, error)

	ArchiveOrphansOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) ([]string, error)
	RestoreOrphansOnLinksAdded(spaceId, contextId string, addedLinks []string) ([]string, error)
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

// ArchiveOrphansOnLinksRemoval checks if any of the removed links are objects that should be garbage collected.
// If onlyBlockIds is provided, it will only process objects created in those specific block IDs.
// It returns the IDs of objects that were archived; the caller is responsible for emitting any events.
func (gc *objectGC) ArchiveOrphansOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) ([]string, error) {
	if len(removedLinks) == 0 {
		return nil, nil
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
		return nil, fmt.Errorf("query objects: %w", err)
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
				log.With("id", id).Errorf("failed to delete object: %v", err)
			}
		} else {
			log.With("id", id).Debugf("archiving orphaned object created in context %s", contextId)
			toArchive = append(toArchive, id)
		}
	}
	if err := gc.objectArchiver.SetListIsArchived(nil, gc.componentCtx, toArchive, true); err != nil {
		return nil, fmt.Errorf("archive objects: %w", err)
	}
	return toArchive, nil
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
// CheckObjectsOnObjectArchived finds objects that should be archived or restored when objectId changes state.
// It returns the IDs of all affected objects; the caller is responsible for emitting any events.
func (gc *objectGC) CheckObjectsOnObjectArchived(spaceId, objectId string, isArchived bool) ([]string, error) {
	log.Debugf("checking objects on object archived: %s isArchived=%v", objectId, isArchived)
	idx := gc.objectStore.SpaceIndex(spaceId)

	d, err := idx.GetDetails(objectId)
	if err != nil {
		return nil, fmt.Errorf("get details of object: %w", err)
	}
	if !slices.Contains(domain.GCEligibleLayouts, model.ObjectTypeLayout(int32(d.GetInt64(bundle.RelationKeyResolvedLayout)))) {
		// system/unsupported objects can't have GC-tracked children
		return nil, nil
	}

	// make sure we have all backlinks updates flushed to the store
	gc.backlinksWatcher.FlushUpdates()

	gcLayouts := makeGCEligibleLayouts()

	if !isArchived {
		return gc.restoreObjectsOnUnarchive(idx, objectId, gcLayouts)
	}
	return gc.archiveOrphanedObjects(idx, objectId, gcLayouts)
}

// collectOrphanedObjects performs a BFS over the createdInContext tree rooted at objectId and
// returns the flat list of object IDs that should be archived alongside it.
//
// It is a pure read operation — no state changes occur. The BFS uses a visited set to detect
// and break cycles in the createdInContext graph. A pending set (always identical to visited)
// tracks IDs that will be archived in this batch; backlinks pointing into the pending set are
// treated as inactive so that sibling cross-references do not falsely prevent archiving.
//
// After the BFS (Query 1 / parent case), a second pass (Query 2 / backlinker case) runs once
// to collect objects that reference objectId as a backlink but have a different parent context,
// applying the same confirmed-inactive parent safety gate as archiveOrphanedObjects previously did.
func (gc *objectGC) collectOrphanedObjects(idx spaceindex.Store, objectId string, gcLayouts []int64) ([]string, error) {
	// visited doubles as the pending set — anything added to the result is also in visited.
	visited := map[string]struct{}{objectId: {}}
	queue := []string{objectId}
	var result []string

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		// Query 1: direct children whose parent context is current.
		childRecords, err := idx.Query(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyCreatedInContext,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.String(current),
				},
				{
					RelationKey: bundle.RelationKeyResolvedLayout,
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       domain.Int64List(gcLayouts),
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("query children of %s: %w", current, err)
		}
		if len(childRecords) == 0 {
			continue
		}

		// Build a candidate map for this BFS level (all non-visited children).
		// Candidates treat each other as "in-batch" for backlink checks, which allows
		// sibling cross-references to be archived together correctly.
		candidates := make(map[string]*domain.Details, len(childRecords))
		for _, record := range childRecords {
			id := record.Details.GetString(bundle.RelationKeyId)
			if _, seen := visited[id]; seen {
				continue
			}
			candidates[id] = record.Details
		}

		// Collect all backlinks that are neither self-references nor already in visited/candidates.
		// We need active-status for these to decide which candidates to exclude.
		linksToCheck := make(map[string]struct{})
		for id, details := range candidates {
			for _, link := range details.GetStringList(bundle.RelationKeyBacklinks) {
				if link == id {
					continue
				}
				if _, inVisited := visited[link]; inVisited {
					continue
				}
				if _, inCandidates := candidates[link]; inCandidates {
					continue
				}
				linksToCheck[link] = struct{}{}
			}
		}

		activeIds, err := gc.queryActiveIds(idx, linksToCheck)
		if err != nil {
			return nil, fmt.Errorf("query active backlinks for children of %s: %w", current, err)
		}

		// Iteratively remove candidates that have active backlinks outside (visited ∪ candidates).
		// This converges because each iteration removes at least one candidate.
		//
		// When candidate X is evicted (has an external active backlink), X remains a live object.
		// We add X to activeIds so that remaining candidates whose only backlink is X will
		// correctly see X as active and also be excluded in the next iteration.
		if activeIds == nil {
			activeIds = make(map[string]struct{})
		}
		for {
			changed := false
			for id, details := range candidates {
				hasExternal := false
				for _, link := range details.GetStringList(bundle.RelationKeyBacklinks) {
					if link == id {
						continue
					}
					if _, inVisited := visited[link]; inVisited {
						continue
					}
					if _, inCandidates := candidates[link]; inCandidates {
						continue // sibling candidate — treated as in-batch
					}
					if _, active := activeIds[link]; active {
						hasExternal = true
						break
					}
				}
				if hasExternal {
					log.Debugf("collectOrphanedObjects: object %s has external active backlinks, skipping subtree", id)
					activeIds[id] = struct{}{} // evicted candidates stay active — visible to remaining candidates
					delete(candidates, id)
					changed = true
					break // restart since candidates changed
				}
			}
			if !changed {
				break
			}
		}

		for id := range candidates {
			visited[id] = struct{}{}
			result = append(result, id)
			queue = append(queue, id)
		}
	}

	// Query 2 (backlinker case): objects where objectId is a backlinker but NOT the parent.
	// Runs once as a post-BFS batch; uses the accumulated pending set when evaluating backlinks.
	backlinkRecords, err := idx.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyBacklinks,
				Condition:   model.BlockContentDataviewFilter_AllIn,
				Value:       domain.StringList([]string{objectId}),
			},
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
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
		return nil, fmt.Errorf("query backlinker objects: %w", err)
	}

	if len(backlinkRecords) > 0 {
		idsToCheck := make(map[string]struct{})
		for _, record := range backlinkRecords {
			id := record.Details.GetString(bundle.RelationKeyId)
			if _, seen := visited[id]; seen {
				continue
			}
			if parentId := record.Details.GetString(bundle.RelationKeyCreatedInContext); parentId != "" {
				idsToCheck[parentId] = struct{}{}
			}
			for _, link := range record.Details.GetStringList(bundle.RelationKeyBacklinks) {
				if link == id || link == objectId {
					continue
				}
				if _, inPending := visited[link]; inPending {
					continue
				}
				idsToCheck[link] = struct{}{}
			}
		}

		activeIds, err := gc.queryActiveIds(idx, idsToCheck)
		if err != nil {
			return nil, fmt.Errorf("query active ids for backlinker objects: %w", err)
		}

		for _, record := range backlinkRecords {
			id := record.Details.GetString(bundle.RelationKeyId)
			if _, seen := visited[id]; seen {
				continue
			}
			parentId := record.Details.GetString(bundle.RelationKeyCreatedInContext)
			if _, parentActive := activeIds[parentId]; parentActive {
				continue
			}
			if !gc.isConfirmedInactive(idx, parentId) {
				log.Debugf("collectOrphanedObjects: object %s parent %s not confirmed inactive, skipping", id, parentId)
				continue
			}
			hasExternal := false
			for _, link := range record.Details.GetStringList(bundle.RelationKeyBacklinks) {
				if link == id || link == objectId {
					continue
				}
				if _, inPending := visited[link]; inPending {
					continue
				}
				if _, active := activeIds[link]; active {
					hasExternal = true
					break
				}
			}
			if hasExternal {
				continue
			}
			visited[id] = struct{}{}
			result = append(result, id)
		}
	}

	return result, nil
}

// archiveOrphanedObjects collects all orphaned objects via BFS and returns their IDs.
// It is a pure read operation — no state changes occur. The caller is responsible for
// archiving the returned IDs and emitting any events.
func (gc *objectGC) archiveOrphanedObjects(idx spaceindex.Store, objectId string, gcLayouts []int64) ([]string, error) {
	toArchive, err := gc.collectOrphanedObjects(idx, objectId, gcLayouts)
	if err != nil {
		return nil, fmt.Errorf("collect orphaned objects: %w", err)
	}
	return toArchive, nil
}

// collectOrphanedForRestore performs a BFS over archived children of objectId and returns the
// flat list of object IDs that should be unarchived alongside it. The BFS queries archived
// objects explicitly (overriding the default isArchived=false implicit filter) and stops when a
// child has backlinks outside the pending set (meaning something else still references it).
func (gc *objectGC) collectOrphanedForRestore(idx spaceindex.Store, objectId string, gcLayouts []int64) ([]string, error) {
	visited := map[string]struct{}{objectId: {}}
	queue := []string{objectId}
	var result []string

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		records, err := idx.Query(database.Query{
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyCreatedInContext,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.String(current),
				},
				{
					RelationKey: bundle.RelationKeyResolvedLayout,
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       domain.Int64List(gcLayouts),
				},
				{
					// Explicit IsArchived == true suppresses the default implicit filter,
					// allowing archived objects to appear in results.
					RelationKey: bundle.RelationKeyIsArchived,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.Bool(true),
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("query archived children of %s: %w", current, err)
		}

		// Build a per-level candidates map so that archived siblings which mutually
		// reference each other are treated as a batch and restored together — mirroring
		// the archive direction's candidates treatment.
		candidates := make(map[string]*domain.Details, len(records))
		for _, record := range records {
			id := record.Details.GetString(bundle.RelationKeyId)
			if _, seen := visited[id]; seen {
				continue
			}
			candidates[id] = record.Details
		}

		// Iteratively remove candidates that have backlinks outside (visited ∪ candidates).
		// Evicted candidates are NOT added to activeIds here because archived objects are
		// not "active" in the usual sense — they are simply not being restored.
		for {
			changed := false
			for id, details := range candidates {
				hasExternal := false
				for _, link := range details.GetStringList(bundle.RelationKeyBacklinks) {
					if link == id {
						continue
					}
					if _, inVisited := visited[link]; inVisited {
						continue
					}
					if _, inCandidates := candidates[link]; inCandidates {
						continue // sibling candidate — treated as in-batch
					}
					hasExternal = true
					break
				}
				if hasExternal {
					log.Debugf("collectOrphanedForRestore: object %s has external backlinks, keeping archived", id)
					delete(candidates, id)
					changed = true
					break
				}
			}
			if !changed {
				break
			}
		}

		for id := range candidates {
			visited[id] = struct{}{}
			result = append(result, id)
			queue = append(queue, id)
		}
	}
	return result, nil
}

// restoreObjectsOnUnarchive collects all archived children via BFS and returns their IDs.
// It is a pure read operation — no state changes occur. The caller is responsible for
// restoring the returned IDs and emitting any events.
func (gc *objectGC) restoreObjectsOnUnarchive(idx spaceindex.Store, objectId string, gcLayouts []int64) ([]string, error) {
	toRestore, err := gc.collectOrphanedForRestore(idx, objectId, gcLayouts)
	if err != nil {
		return nil, fmt.Errorf("collect orphaned for restore: %w", err)
	}
	return toRestore, nil
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

// RestoreOrphansOnLinksAdded unarchives objects that were previously GC'd when links are re-added
// to a context object (e.g. via undo). For each added link that is an archived object whose
// CreatedInContext matches the context, the object is restored unconditionally — the active link
// in the page is sufficient justification to unarchive it.
// RestoreOrphansOnLinksAdded returns the IDs of objects that were restored; the caller emits events.
func (gc *objectGC) RestoreOrphansOnLinksAdded(spaceId, contextId string, addedLinks []string) ([]string, error) {
	if len(addedLinks) == 0 {
		return nil, nil
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
		return nil, fmt.Errorf("query archived objects for restore: %w", err)
	}

	var toRestore []string
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		log.Debugf("restoring archived object %s after link re-added to context %s", id, contextId)
		toRestore = append(toRestore, id)
	}
	if err := gc.objectArchiver.SetListIsArchived(nil, gc.componentCtx, toRestore, false); err != nil {
		return nil, fmt.Errorf("restore objects: %w", err)
	}
	return toRestore, nil
}

// isConfirmedInactive returns true only if id is explicitly indexed in the store with
// isArchived=true or isDeleted=true. Returns false if id is missing from the store
// (sync gap) or if it is still active. Used to guard against archiving objects whose
// parent context has not yet been delivered by sync.
func (gc *objectGC) isConfirmedInactive(idx spaceindex.Store, id string) bool {
	if id == "" {
		return false
	}
	d, err := idx.GetDetails(id)
	if err != nil {
		return false // not indexed — treat as unknown, not confirmed inactive
	}
	return d.GetBool(bundle.RelationKeyIsArchived) || d.GetBool(bundle.RelationKeyIsDeleted)
}

func makeGCEligibleLayouts() []int64 {
	layouts := make([]int64, 0, len(domain.GCEligibleLayouts))
	for _, layout := range domain.GCEligibleLayouts {
		layouts = append(layouts, int64(layout))
	}
	return layouts
}

// FilterExplicitIds removes ids from all auto-archive and auto-restore events in sctx.
// Call this after GC runs to prevent explicitly user-requested IDs from appearing in
// auto events — e.g. if B is in the original archive request AND gets auto-archived
// as a child of A, it should not be reported as auto-archived.
func FilterExplicitIds(sctx session.Context, ids []string) {
	if sctx == nil || len(ids) == 0 {
		return
	}
	exclude := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		exclude[id] = struct{}{}
	}
	msgs := sctx.GetMessages()
	changed := false
	result := make([]*pb.EventMessage, 0, len(msgs))
	for _, msg := range msgs {
		switch v := msg.Value.(type) {
		case *pb.EventMessageValueOfObjectAutoArchive:
			filtered := filterExcluded(v.ObjectAutoArchive.ObjectIds, exclude)
			if len(filtered) == len(v.ObjectAutoArchive.ObjectIds) {
				result = append(result, msg)
				continue
			}
			changed = true
			if len(filtered) > 0 {
				result = append(result, &pb.EventMessage{
					Value: &pb.EventMessageValueOfObjectAutoArchive{
						ObjectAutoArchive: &pb.EventObjectAutoArchive{ObjectIds: filtered},
					},
				})
			}
		case *pb.EventMessageValueOfObjectAutoRestore:
			filtered := filterExcluded(v.ObjectAutoRestore.ObjectIds, exclude)
			if len(filtered) == len(v.ObjectAutoRestore.ObjectIds) {
				result = append(result, msg)
				continue
			}
			changed = true
			if len(filtered) > 0 {
				result = append(result, &pb.EventMessage{
					Value: &pb.EventMessageValueOfObjectAutoRestore{
						ObjectAutoRestore: &pb.EventObjectAutoRestore{ObjectIds: filtered},
					},
				})
			}
		default:
			result = append(result, msg)
		}
	}
	if changed {
		sctx.SetMessages(sctx.ObjectID(), result)
	}
}

func filterExcluded(ids []string, exclude map[string]struct{}) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, skip := exclude[id]; !skip {
			out = append(out, id)
		}
	}
	return out
}
