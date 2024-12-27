package subscription

import (
	"github.com/anyproto/anytype-heart/core/domain"
)

func (s *spaceSubscriptions) newSimpleSub(id string, spaceId string, keys []domain.RelationKey, isDep bool) *simpleSub {
	sub := &simpleSub{
		id:      id,
		spaceId: spaceId,
		keys:    keys,
		cache:   s.cache,
	}
	if !isDep {
		sub.ds = s.ds
	}
	return sub
}

type simpleSub struct {
	id       string
	spaceId  string
	set      map[string]struct{}
	keys     []domain.RelationKey
	forceIds []string

	depKeys          []domain.RelationKey
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
		e.SetSub(s.id, true, true)
	}
	if s.ds != nil {
		s.depKeys = s.ds.depKeys(s.spaceId, s.keys)
		if len(s.depKeys) > 0 {
			s.depSub = s.ds.makeSubscriptionByEntries(s.id+"/dep", s.spaceId, entries, s.getActiveEntries(), s.keys, s.depKeys, nil)
		}
	}
	return
}

func (s *simpleSub) isEqualIds(ids []string) bool {
	if len(s.set) != len(ids) {
		return false
	}
	for _, id := range ids {
		if _, ok := s.set[id]; !ok {
			return false
		}
	}
	return true
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
			ctx.position = append(ctx.position, opPosition{
				id:    e.id,
				subId: s.id,
				keys:  s.keys,
				isAdd: true,
			})
		}
		newSet[e.id] = struct{}{}
		e.SetSub(s.id, true, false)
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
			e.SetSub(s.id, true, false)
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

func (s *simpleSub) getActiveRecords() (res []*domain.Details) {
	for id := range s.set {
		res = append(res, s.cache.Get(id).data.CopyOnlyKeys(s.keys...))
	}
	return
}

func (s *simpleSub) hasDep() bool {
	return s.depSub != nil
}

func (s *simpleSub) getDep() subscription {
	return s.depSub
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
