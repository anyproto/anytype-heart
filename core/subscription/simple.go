package subscription

import (
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

func (s *service) newSimpleSub(id string, keys []string, isDep bool) *simpleSub {
	sub := &simpleSub{
		id:    id,
		keys:  keys,
		cache: s.cache,
	}
	if !isDep {
		sub.ds = s.ds
	}
	return sub
}

type simpleSub struct {
	id   string
	set  map[string]struct{}
	keys []string

	depKeys          []string
	depSub           *simpleSub
	activeEntriesBuf []*entry

	cache *cache
	ds    *dependencyService
}

func (s *simpleSub) init(entries []*entry) (err error) {
	s.set = make(map[string]struct{})
	for _, e := range entries {
		e = s.cache.GetOrSet(e)
		s.set[e.id] = struct{}{}
		e.AddSubId(s.id, true)
	}
	if s.ds != nil {
		s.depKeys = s.ds.depKeys(s.keys)
		if len(s.depKeys) > 0 {
			s.depSub = s.ds.makeSubscriptionByEntries(s.id+"/dep", entries, s.getActiveEntries(), s.keys, s.depKeys)
		}
	}
	return
}

func (s *simpleSub) refill(ctx *opCtx, entries []*entry) {
	var newSet = make(map[string]struct{})
	for _, e := range entries {
		if _, inSet := s.set[e.id]; inSet {
			ctx.change = append(ctx.change, opChange{
				id:    e.id,
				subId: s.id,
				keys:  s.keys,
			})
		} else {
			ctx.add = append(ctx.add, opChange{
				id:    e.id,
				subId: s.id,
				keys:  s.keys,
			})
		}
		newSet[e.id] = struct{}{}
		e.AddSubId(s.id, true)
	}
	for oldId := range s.set {
		if _, inSet := newSet[oldId]; !inSet {
			ctx.remove = append(ctx.remove, opRemove{
				id:    oldId,
				subId: s.id,
			})
			s.cache.RemoveSubId(oldId, s.id)
		}
	}
	s.set = newSet
}

func (s *simpleSub) counters() (prev, next int) {
	return 0, 0
}

func (s *simpleSub) onChange(ctx *opCtx) {
	var changed bool
	for _, e := range ctx.entries {
		if _, inSet := s.set[e.id]; inSet {
			ctx.change = append(ctx.change, opChange{
				id:    e.id,
				subId: s.id,
				keys:  s.keys,
			})
			changed = true
			e.AddSubId(s.id, true)
		}
	}
	if changed && s.depSub != nil {
		s.ds.refillSubscription(ctx, s.depSub, s.getActiveEntries(), s.depKeys)
	}
}

func (s *simpleSub) getActiveEntries() (res []*entry) {
	s.activeEntriesBuf = s.activeEntriesBuf[:0]
	for id := range s.set {
		res = append(res, s.cache.Get(id))
	}
	return s.activeEntriesBuf
}

func (s *simpleSub) getActiveRecords() (res []*types.Struct) {
	for id := range s.set {
		res = append(res, pbtypes.StructFilterKeys(s.cache.Get(id).data, s.keys))
	}
	return
}

func (s *simpleSub) close() {
	for id := range s.set {
		s.cache.RemoveSubId(id, s.id)
	}
	if s.depSub != nil {
		s.depSub.close()
	}
	return
}
