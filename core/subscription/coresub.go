package subscription

import (
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

// coreSub is the engine's single subscription primitive: an id scope (later
// phases), compiled filters, an optional order+window (later phases) and the
// requested keys, maintained as a member set over one space's feed.
//
// All state is private to the sub and mutated only under its space's mutex.
// Every *domain.Details held here is frozen: state updates replace pointers,
// never mutate maps, because member projections double as event payloads.
type coreSub struct {
	subId   string
	spaceId string
	keys    []domain.RelationKey
	filters *database.Filters

	// members maps id → projection of the latest details to the requested
	// keys. For unordered subs every member is visible, so the projection
	// doubles as the prev-state for diffing.
	members map[string]*domain.Details

	// countersDirty marks that membership changed within the current batch;
	// finalize() turns it into a single trailing Counters op
	countersDirty bool

	// detailEventsOnly suppresses Add/Remove/Position/Counters (dep subs)
	detailEventsOnly bool

	// queue is the delivery target for internal subs; nil means broadcast.
	// queueOwned distinguishes engine-created queues (closed on teardown)
	// from caller-provided ones (never touched, e.g. crossspacesub's shared
	// queue)
	queue      *mb.MB[*pb.EventMessage]
	queueOwned bool

	space *spaceState
}

// apply evaluates one feed update against the sub and appends the resulting
// transition ops. details == nil means the object is gone from the store
// (hard delete observed via re-fetch miss) and is treated as a non-match.
// Runs under the space mutex.
func (c *coreSub) apply(id string, details *domain.Details, out *opBatch) {
	matched := details != nil && c.filters.FilterObj != nil && c.filters.FilterObj.FilterObject(details)
	prev, isMember := c.members[id]
	switch {
	case matched && !isMember:
		proj := projectDetails(details, c.keys)
		c.members[id] = proj
		c.countersDirty = true
		out.append(subOp{sub: c, kind: opSet, id: id, details: proj})
		if !c.detailEventsOnly {
			out.append(subOp{sub: c, kind: opAdd, id: id})
		}
	case !matched && isMember:
		delete(c.members, id)
		c.countersDirty = true
		if !c.detailEventsOnly {
			out.append(subOp{sub: c, kind: opRemove, id: id})
		}
	case matched && isMember:
		next, amend, unset := diffProject(prev, details, c.keys)
		if next == nil {
			return
		}
		c.members[id] = next
		if len(amend) > 0 {
			out.append(subOp{sub: c, kind: opAmend, id: id, amend: amend})
		}
		if len(unset) > 0 {
			out.append(subOp{sub: c, kind: opUnset, id: id, unset: unset})
		}
	}
}

// finalize emits the per-batch trailing Counters op when membership changed
func (c *coreSub) finalize(out *opBatch) {
	if !c.countersDirty {
		return
	}
	c.countersDirty = false
	if c.detailEventsOnly {
		return
	}
	out.append(subOp{sub: c, kind: opCounters, total: int64(len(c.members))})
}

// memberIds snapshots the current member id set. Runs under the space mutex.
func (c *coreSub) memberIds() []string {
	ids := make([]string, 0, len(c.members))
	for id := range c.members {
		ids = append(ids, id)
	}
	return ids
}

// projectDetails copies only the requested keys out of full details
func projectDetails(details *domain.Details, keys []domain.RelationKey) *domain.Details {
	proj := domain.NewDetails()
	for _, k := range keys {
		if v, ok := details.TryGet(k); ok {
			proj.Set(k, v)
		}
	}
	return proj
}

// diffProject compares the previous projection against fresh full details in
// a single pass over the requested keys. It returns the new projection plus
// the changed and removed keys, or (nil, nil, nil) when nothing changed —
// allocating only on the first detected difference, which keeps the no-op
// path (snapshot/feed overlap, storms of irrelevant changes) allocation-free.
func diffProject(prev, details *domain.Details, keys []domain.RelationKey) (next *domain.Details, amend []amendKV, unset []string) {
	for _, k := range keys {
		nv, nok := details.TryGet(k)
		pv, pok := prev.TryGet(k)
		if nok && (!pok || !nv.Equal(pv)) {
			amend = append(amend, amendKV{key: string(k), value: nv})
		} else if !nok && pok {
			unset = append(unset, string(k))
		}
	}
	if amend == nil && unset == nil {
		return nil, nil, nil
	}
	next = prev.Copy()
	for _, kv := range amend {
		next.Set(domain.RelationKey(kv.key), kv.value)
	}
	for _, k := range unset {
		next.Delete(domain.RelationKey(k))
	}
	return next, amend, unset
}
