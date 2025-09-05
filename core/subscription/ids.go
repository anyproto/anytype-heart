package subscription

import (
	"github.com/anyproto/anytype-heart/core/domain"
)

func (s *spaceSubscriptions) newIdsSub(id string, keys []domain.RelationKey, isDep bool) *idsSub {
	sub := &idsSub{
		id:       id,
		keys:     keys,
		cache:    s.cache,
		entryMap: make(map[string]*entry),
	}
	if !isDep {
		sub.ds = s.ds
	}
	return sub
}

type idsSub struct {
	id   string
	keys []domain.RelationKey

	started              bool
	entriesBeforeStarted []*entry

	entryMap map[string]*entry

	depKeys          []domain.RelationKey
	depSub           *simpleSub
	activeEntriesBuf []*entry

	cache *cache
	ds    *dependencyService
}

func (s *idsSub) init(entries []*entry) (err error) {
	s.started = true

	for _, e := range entries {
		e = s.cache.GetOrSet(e)
		s.entryMap[e.id] = e
		e.SetSub(s.id, true, true)
	}

	if s.ds != nil {
		s.depKeys = s.ds.depKeys(s.keys)
		if len(s.depKeys) > 0 {
			s.depSub = s.ds.makeSubscriptionByEntries(s.id+"/dep", entries, s.getActiveEntries(), s.keys, s.depKeys, nil)
		}
	}
	return
}

func (s *idsSub) counters() (prev, next int) {
	return 0, 0
}

func (s *idsSub) onChange(ctx *opCtx) {
	var changed bool

	for _, e := range ctx.entries {
		// If subscription hasn't started yet, accumulate entries for later processing
		if !s.started {
			if _, exists := s.entryMap[e.id]; exists {
				s.entriesBeforeStarted = append(s.entriesBeforeStarted, e)
			}
			continue
		}

		// Check if this entry is one we're tracking
		if _, exists := s.entryMap[e.id]; exists {
			// Update the entry in our map (or set it if it was nil/not available before)
			oldEntry := s.entryMap[e.id]
			s.entryMap[e.id] = e

			if oldEntry == nil {
				ctx.position = append(ctx.position, opPosition{
					id:    e.id,
					subId: s.id,
					keys:  s.keys,
					isAdd: true,
				})
				// To send details
				ctx.change = append(ctx.change, opChange{
					id:    e.id,
					subId: s.id,
					keys:  s.keys,
				})
			} else {
				ctx.change = append(ctx.change, opChange{
					id:    e.id,
					subId: s.id,
					keys:  s.keys,
				})
			}
			changed = true
			e.SetSub(s.id, true, false)
		}
	}

	if changed && s.depSub != nil {
		activeEntries := s.getActiveEntries()
		s.ds.refillSubscription(ctx, s.depSub, activeEntries, s.depKeys)
		s.ds.updateOrders(s.id, activeEntries)
	}
}

func (s *idsSub) getActiveEntries() (res []*entry) {
	s.activeEntriesBuf = s.activeEntriesBuf[:0]
	for _, entry := range s.entryMap {
		if entry != nil {
			s.activeEntriesBuf = append(s.activeEntriesBuf, entry)
		}
	}
	return s.activeEntriesBuf
}

func (s *idsSub) getActiveRecords() (res []*domain.Details) {
	for _, entry := range s.entryMap {
		if entry != nil {
			res = append(res, entry.data.CopyOnlyKeys(s.keys...))
		}
	}
	return
}

func (s *idsSub) hasDep() bool {
	return s.depSub != nil
}

func (s *idsSub) getDep() subscription {
	return s.depSub
}

func (s *idsSub) close() {
	for id := range s.entryMap {
		s.cache.RemoveSubId(id, s.id)
	}
	if s.depSub != nil {
		s.depSub.close()
	}
}

func (s *idsSub) addIds(ids []string) {
	for _, id := range ids {
		if _, exists := s.entryMap[id]; !exists {
			// Check if the entry is already in cache
			if cachedEntry := s.cache.Get(id); cachedEntry != nil {
				s.entryMap[id] = cachedEntry
				cachedEntry.SetSub(s.id, true, false)
			} else {
				// Object not yet available - reserve slot in map
				s.entryMap[id] = nil
			}
		}
	}
}
