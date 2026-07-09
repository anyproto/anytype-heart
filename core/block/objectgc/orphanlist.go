package objectgc

import (
	"fmt"
	"sort"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// OrphanReason explains why a root is orphaned. It is set on roots only; descendants carry
// OrphanReasonNone because the reason belongs to their root.
type OrphanReason int

const (
	OrphanReasonNone OrphanReason = iota
	OrphanReasonContextArchived
	OrphanReasonContextDeleted
	OrphanReasonContextUnlinked
)

// OrphanItem is one member of the removable set.
type OrphanItem struct {
	Details *domain.Details
	IsRoot  bool
	Reason  OrphanReason
}

// parentState is the store state of a candidate's createdInContext parent.
type parentState int

const (
	parentAbsent parentState = iota // not indexed at all → never synced (deletion tombstones)
	parentArchived
	parentDeleted
	parentActive
)

// queryParentStates resolves existence + archived/deleted state for the given ids using QueryRaw,
// which (unlike Query) applies no implicit isArchived/isDeleted filters. Ids not returned are absent.
func (gc *objectGC) queryParentStates(idx spaceindex.Store, ids map[string]struct{}) (map[string]parentState, error) {
	states := make(map[string]parentState, len(ids))
	if len(ids) == 0 {
		return states, nil
	}
	values := make([]domain.Value, 0, len(ids))
	for id := range ids {
		states[id] = parentAbsent
		values = append(values, domain.String(id))
	}
	records, err := idx.QueryRaw(&database.Filters{FilterObj: database.FilterIn{
		Key:   bundle.RelationKeyId,
		Value: values,
	}}, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("query parent states: %w", err)
	}
	for _, r := range records {
		id := r.Details.GetString(bundle.RelationKeyId)
		switch {
		case r.Details.GetBool(bundle.RelationKeyIsDeleted):
			states[id] = parentDeleted
		case r.Details.GetBool(bundle.RelationKeyIsArchived):
			states[id] = parentArchived
		default:
			states[id] = parentActive
		}
	}
	return states, nil
}

// ListOrphans computes the space's orphan forest.
//
// Candidates: active objects with createdInContext + non-empty createdInContextRef, a GC-eligible
// layout, not createdInContextIgnored, and whose parent is present in the store (sync-gap guard).
//
// The removable set S is the greatest subset of the candidates such that every member's *active*
// backlinks fall inside S. Anything reachable from an active object outside S is evicted, and that
// eviction cascades. Roots are the members whose parent is outside S.
func (gc *objectGC) ListOrphans(spaceId string) ([]OrphanItem, error) {
	gc.backlinksWatcher.FlushUpdates()
	idx := gc.objectStore.SpaceIndex(spaceId)

	// 1) Candidates. Query injects the implicit isArchived/isDeleted filters, so these are active.
	records, err := idx.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyCreatedInContext,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyCreatedInContextRef,
				Condition:   model.BlockContentDataviewFilter_NotEmpty, // collections have empty ref
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(makeGCEligibleLayouts()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query orphan candidates: %w", err)
	}

	candidates := make(map[string]*domain.Details, len(records))
	for _, r := range records {
		// The ignore gate is applied in memory to avoid depending on NotEqual-vs-missing-key
		// filter semantics.
		if r.Details.GetBool(bundle.RelationKeyCreatedInContextIgnored) {
			continue
		}
		candidates[r.Details.GetString(bundle.RelationKeyId)] = r.Details
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// isCandidate is fixed here, before the sync-gap guard mutates candidates. The guard must decide
	// "is my parent a candidate" against the original set, otherwise its answer depends on map
	// iteration order: a child visited after its dropped parent would see the parent as external.
	isCandidate := make(map[string]struct{}, len(candidates))
	for id := range candidates {
		isCandidate[id] = struct{}{}
	}

	// 2) Parent states, for the sync-gap guard and the per-root reason. Parents that are themselves
	// candidates are active by construction and need no lookup.
	toLookup := make(map[string]struct{})
	for _, d := range candidates {
		p := d.GetString(bundle.RelationKeyCreatedInContext)
		if _, ok := isCandidate[p]; !ok {
			toLookup[p] = struct{}{}
		}
	}
	parentStates, err := gc.queryParentStates(idx, toLookup)
	if err != nil {
		return nil, fmt.Errorf("resolve parent states: %w", err)
	}

	// Sync-gap guard: a parent absent from the store was never synced (deletion tombstones the row),
	// so we must not recommend removing its children.
	for id, d := range candidates {
		p := d.GetString(bundle.RelationKeyCreatedInContext)
		if _, ok := isCandidate[p]; ok {
			continue
		}
		if parentStates[p] == parentAbsent {
			delete(candidates, id)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	// 3) Active status of every backlink target that is not itself a candidate. Candidates dropped by
	// the sync-gap guard are looked up here too: they are live objects outside the set, so a child
	// backlinked only by such a parent is correctly evicted below.
	linksToCheck := make(map[string]struct{})
	for id, d := range candidates {
		for _, b := range d.GetStringList(bundle.RelationKeyBacklinks) {
			if b == id {
				continue
			}
			if _, ok := candidates[b]; ok {
				continue
			}
			linksToCheck[b] = struct{}{}
		}
	}
	activeIds, err := gc.queryActiveIds(idx, linksToCheck)
	if err != nil {
		return nil, fmt.Errorf("query active backlinks: %w", err)
	}

	inS := gc.evictToFixedPoint(candidates, activeIds)
	if len(inS) == 0 {
		return nil, nil
	}
	return gc.buildForest(candidates, inS, parentStates), nil
}

// evictToFixedPoint returns the greatest subset of candidates whose active backlinks all fall
// inside the subset. Worklist algorithm: O(V+E), not a naive restart loop.
func (gc *objectGC) evictToFixedPoint(candidates map[string]*domain.Details, activeIds map[string]struct{}) map[string]struct{} {
	inS := make(map[string]struct{}, len(candidates))
	for id := range candidates {
		inS[id] = struct{}{}
	}

	// reverse index: backlink target -> candidates that list it as a backlink
	revIdx := make(map[string][]string)
	for id, d := range candidates {
		for _, b := range d.GetStringList(bundle.RelationKeyBacklinks) {
			if b == id {
				continue
			}
			revIdx[b] = append(revIdx[b], id)
		}
	}

	// Seed: evict candidates that have an active backlink outside the candidate set.
	var queue []string
	for id, d := range candidates {
		for _, b := range d.GetStringList(bundle.RelationKeyBacklinks) {
			if b == id {
				continue
			}
			if _, ok := candidates[b]; ok {
				continue
			}
			if _, active := activeIds[b]; active {
				queue = append(queue, id)
				break
			}
		}
	}

	// Propagate: an evicted candidate is active and now outside S, so anything backlinking it
	// must be evicted too.
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := inS[id]; !ok {
			continue
		}
		delete(inS, id)
		for _, x := range revIdx[id] {
			if _, ok := inS[x]; ok {
				queue = append(queue, x)
			}
		}
	}
	return inS
}

// buildForest marks roots (parent outside S), resolves each root's reason, and handles
// createdInContext cycles (a component with no parent-outside-S) by electing the lowest id as root.
func (gc *objectGC) buildForest(candidates map[string]*domain.Details, inS map[string]struct{}, parentStates map[string]parentState) []OrphanItem {
	parentOf := func(id string) string {
		return candidates[id].GetString(bundle.RelationKeyCreatedInContext)
	}

	children := make(map[string][]string)
	var roots []string
	for id := range inS {
		p := parentOf(id)
		if _, ok := inS[p]; ok {
			children[p] = append(children[p], id)
		} else {
			roots = append(roots, id)
		}
	}

	visited := make(map[string]struct{}, len(inS))
	var markReachable func(from string)
	markReachable = func(from string) {
		if _, ok := visited[from]; ok {
			return
		}
		visited[from] = struct{}{}
		for _, c := range children[from] {
			markReachable(c)
		}
	}
	for _, r := range roots {
		markReachable(r)
	}

	// Anything unvisited sits in (or below) a createdInContext cycle: its parent chain never
	// leaves S. Elect the lowest id on the cycle as the root so rendering is deterministic.
	var leftover []string
	for id := range inS {
		if _, ok := visited[id]; !ok {
			leftover = append(leftover, id)
		}
	}
	sort.Strings(leftover)
	for _, start := range leftover {
		if _, ok := visited[start]; ok {
			continue
		}
		// Walk parents until we revisit a node in this walk — those nodes form the cycle.
		seen := map[string]int{}
		cur := start
		for {
			if _, ok := seen[cur]; ok {
				break
			}
			seen[cur] = len(seen)
			cur = parentOf(cur)
		}
		cycleStart := seen[cur]
		cycle := make([]string, 0, len(seen))
		for id, order := range seen {
			if order >= cycleStart {
				cycle = append(cycle, id)
			}
		}
		sort.Strings(cycle)
		root := cycle[0]
		roots = append(roots, root)
		markReachable(root)
	}

	rootSet := make(map[string]struct{}, len(roots))
	for _, r := range roots {
		rootSet[r] = struct{}{}
	}

	reasonFor := func(id string) OrphanReason {
		p := parentOf(id)
		if _, ok := candidates[p]; ok {
			// Parent is a candidate, so it is active. Either it was evicted from S — and then it
			// cannot link this object, or this object would have been evicted with it — or it is a
			// peer on this object's createdInContext cycle, which nothing outside the cycle links.
			// Both mean: the context is alive but no longer references this object.
			return OrphanReasonContextUnlinked
		}
		switch parentStates[p] {
		case parentArchived:
			return OrphanReasonContextArchived
		case parentDeleted:
			return OrphanReasonContextDeleted
		default:
			return OrphanReasonContextUnlinked
		}
	}

	items := make([]OrphanItem, 0, len(inS))
	for id := range inS {
		item := OrphanItem{Details: candidates[id]}
		if _, ok := rootSet[id]; ok {
			item.IsRoot = true
			item.Reason = reasonFor(id)
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Details.GetString(bundle.RelationKeyId) < items[j].Details.GetString(bundle.RelationKeyId)
	})
	return items
}
