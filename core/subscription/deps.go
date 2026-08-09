package subscription

import (
	"slices"
	"sort"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// depTracker maintains a parent subscription's render dependencies: the
// objects referenced by dep-format values (see isDepFormat) among the
// requested keys of visible members, plus objects referenced by filter
// values. They are delivered through a hidden child sub under "{subId}/dep"
// — a scoped, detail-events-only sibling registered in the same space, so
// dep detail changes flow through the regular pipeline. The dep id set is
// recomputed after batches that touched the parent's membership, window or
// dep-key values (parent.depDirty).
//
// Order dependencies (sorting by an object/tag/status relation) are handled
// separately and cheaper: every feed item is offered to the parent's
// compiled order via UpdateOrderMap — the order map already knows which ids
// it depends on — and a reported change triggers a window re-query
// (coreSub.checkOrderDep).
type depTracker struct {
	parent       *coreSub
	child        *coreSub
	depKeys      []domain.RelationKey
	depKeySet    map[string]struct{}
	filterDepIds []string
}

// isDepFormat reports whether values of this format are ids of objects the
// client must resolve to render. tag and status belong here as much as
// object and file do: their values are relationOption ids, and clients build
// the chip from the option's name and colour.
func isDepFormat(format model.RelationFormat) bool {
	switch format {
	case model.RelationFormat_object, model.RelationFormat_file, model.RelationFormat_tag, model.RelationFormat_status:
		return true
	default:
		return false
	}
}

// newDepTracker resolves which requested keys carry object references and
// which filter values are object ids; returns nil when the request cannot
// produce dependencies
func newDepTracker(parent *coreSub, spec subSpec, idx spaceindex.Store) *depTracker {
	var depKeys []domain.RelationKey
	for _, k := range spec.keys {
		if k == bundle.RelationKeyId {
			continue
		}
		format, err := idx.GetRelationFormatByKey(k)
		if err != nil {
			continue
		}
		if isDepFormat(format) {
			depKeys = append(depKeys, k)
		}
	}
	filterDepIds := collectFilterDepIds(spec.filters, idx)
	if len(depKeys) == 0 && len(filterDepIds) == 0 {
		return nil
	}
	keySet := make(map[string]struct{}, len(depKeys))
	for _, k := range depKeys {
		keySet[string(k)] = struct{}{}
	}
	child := &coreSub{
		subId:   spec.subId + "/dep",
		spaceId: spec.spaceId,
		// dependent objects are rendered as name/icon chips; never stream the
		// high-churn strip-by-default keys (sync/usage) for them, even if the
		// parent lists those keys for its own rows. depKeys above are derived
		// from spec.keys and are unaffected (stripped keys never carry a dep
		// format).
		keys:             slices.DeleteFunc(slices.Clone(spec.keys), bundle.IsDefaultStrippedKey),
		members:          make(map[string]struct{}),
		vis:              make(map[string]*visEntry),
		detailEventsOnly: true,
		isDepChild:       true,
		queue:            parent.queue,
		// mirror the parent's ownership so dep traffic counts against the
		// queue overflow watermark; teardown still closes the queue only
		// through the parent, so no double-close
		queueOwned: parent.queueOwned,
	}
	child.setScopeIds(nil)
	return &depTracker{
		parent:       parent,
		child:        child,
		depKeys:      depKeys,
		depKeySet:    keySet,
		filterDepIds: filterDepIds,
	}
}

// collectFilterDepIds extracts object ids the request filters by (the client
// renders the names of objects in its filter values), recursing into
// And/Or trees
func collectFilterDepIds(filters []database.FilterRequest, idx spaceindex.Store) []string {
	var ids []string
	seen := make(map[string]struct{})
	var walk func(fs []database.FilterRequest)
	walk = func(fs []database.FilterRequest) {
		for _, f := range fs {
			// mirror the compiler's branch/leaf discriminator: an Operator
			// makes it a branch (leaf fields ignored), otherwise it is a
			// leaf even if NestedFilters is populated
			if f.Operator != model.BlockContentDataviewFilter_No {
				walk(f.NestedFilters)
				continue
			}
			if f.Condition == model.BlockContentDataviewFilter_None {
				// disabled leaf: the compiler drops it, so its values never
				// filter anything and must not become deps
				continue
			}
			if f.RelationKey == "" || f.RelationKey == bundle.RelationKeyId {
				continue
			}
			format, err := idx.GetRelationFormatByKey(f.RelationKey)
			if err != nil || !isDepFormat(format) {
				continue
			}
			for _, id := range f.Value.WrapToStringList() {
				if id == "" {
					continue
				}
				if _, ok := seen[id]; ok {
					continue
				}
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	walk(filters)
	return ids
}

// computeDepIds derives the dep id set from the parent's visible members'
// dep-key values plus the filter-value deps, excluding objects the parent
// already delivers itself. Sorted for deterministic event order. Runs under
// the space mutex.
func (d *depTracker) computeDepIds() []string {
	set := make(map[string]struct{}, len(d.filterDepIds))
	add := func(id string) {
		if id == "" {
			return
		}
		if _, visible := d.parent.vis[id]; visible {
			return
		}
		set[id] = struct{}{}
	}
	for _, e := range d.parent.vis {
		for _, k := range d.depKeys {
			for _, id := range e.prev.Get(k).WrapToStringList() {
				add(id)
			}
		}
	}
	for _, id := range d.filterDepIds {
		add(id)
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// amendTouchesDepKeys reports whether a detail diff affects dep-relevant keys
func (d *depTracker) amendTouchesDepKeys(amend []amendKV, unset []string) bool {
	for _, kv := range amend {
		if _, ok := d.depKeySet[kv.key]; ok {
			return true
		}
	}
	for _, k := range unset {
		if _, ok := d.depKeySet[k]; ok {
			return true
		}
	}
	return false
}
