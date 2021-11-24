package subscription

import "github.com/gogo/protobuf/types"

func (s *service) newSimpleSub(id string, keys []string) *simpleSub {
	return &simpleSub{
		id:    id,
		keys:  keys,
		cache: s.cache,
		ds:    s.ds,
	}
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
		s.cache.set(e)
		s.set[e.id] = struct{}{}
		s.cache.get(e.id)
	}
	if s.ds != nil {
		s.depKeys = s.ds.depKeys(s.keys)
		if len(s.depKeys) > 0 {
			s.depSub = s.ds.makeSubscriptionByEntries(s.id+"/dep", s.getActiveEntries(), s.keys, s.depKeys)
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
			s.cache.set(e)
			s.cache.get(e.id)
		}
		newSet[e.id] = struct{}{}
	}
	for oldId := range s.set {
		if _, inSet := newSet[oldId]; !inSet {
			ctx.remove = append(ctx.remove, opRemove{
				id:    oldId,
				subId: s.id,
			})
			s.cache.release(oldId)
		}
	}
}

func (s *simpleSub) counters() (prev, next int) {
	return 0, 0
}

func (s *simpleSub) onChangeBatch(ctx *opCtx, entries ...*entry) {
	var changed bool
	for _, e := range entries {
		if _, inSet := s.set[e.id]; inSet {
			ctx.change = append(ctx.change, opChange{
				id:    e.id,
				subId: s.id,
				keys:  s.keys,
			})
			changed = true
		}
	}
	if changed && s.depSub != nil {
		s.ds.refillSubscription(ctx, s.depSub, s.getActiveEntries(), s.depKeys)
	}
}

func (s *simpleSub) getActiveEntries() (res []*entry) {
	s.activeEntriesBuf = s.activeEntriesBuf[:0]
	for id := range s.set {
		res = append(res, s.cache.pick(id))
	}
	return s.activeEntriesBuf
}

func (s *simpleSub) getActiveRecords() (res []*types.Struct) {
	for id := range s.set {
		res = append(res, s.cache.pick(id).data)
	}
	return
}

func (s *simpleSub) close() {
	for id := range s.set {
		s.cache.release(id)
	}
	return
}
