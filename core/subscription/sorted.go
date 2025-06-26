package subscription

import (
	"errors"

	"github.com/huandu/skiplist"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

var (
	ErrAfterId   = errors.New("after id not in set")
	ErrBeforeId  = errors.New("before id not in set")
	ErrNoRecords = errors.New("no records with given offset")
)

func (s *spaceSubscriptions) newSortedSub(id string, spaceId string, keys []domain.RelationKey, filter database.Filter, order database.Order, limit, offset int) *sortedSub {
	sub := &sortedSub{
		id:          id,
		spaceId:     spaceId,
		keys:        keys,
		filter:      filter,
		order:       order,
		cache:       s.cache,
		ds:          s.ds,
		limit:       limit,
		offset:      offset,
		objectStore: s.objectStore,
	}
	return sub
}

type sortedSub struct {
	id      string
	spaceId string

	started              bool
	entriesBeforeStarted []*entry

	keys   []domain.RelationKey
	filter database.Filter
	order  database.Order

	afterId, beforeId string
	limit, offset     int

	skl               *skiplist.SkipList
	afterEl, beforeEl *skiplist.Element

	depSub           *simpleSub
	depKeys          []domain.RelationKey
	activeEntriesBuf []*entry

	forceSubIds []string
	disableDep  bool

	diff *listDiff

	compCountBefore, compCountAfter opCounter

	cache *cache
	ds    *dependencyService

	// for nested subscriptions
	objectStore spaceindex.Store
	// parent is used to run onChange callback when any child subscriptions receive changes
	parent       *sortedSub
	parentFilter *database.FilterNestedIn
	// nested is used to close child subscriptions when parent is closed
	nested []*sortedSub
}

func (s *sortedSub) init(entries []*entry) (err error) {
	s.skl = skiplist.New(s)
	s.started = true

	defer func() {
		if err != nil {
			s.close()
		}
	}()
	for i, e := range entries {
		e = s.cache.GetOrSet(e)
		entries[i] = e
		e.SetSub(s.id, false, false)
		s.skl.Set(e, nil)
	}
	if s.afterId != "" {
		e := s.cache.Get(s.afterId)
		if e == nil {
			err = ErrAfterId
			return
		}
		s.afterEl = s.skl.Get(e)
		if s.afterEl == nil {
			err = ErrAfterId
			return
		}
	} else if s.beforeId != "" {
		e := s.cache.Get(s.beforeId)
		if e == nil {
			err = ErrBeforeId
			return
		}
		s.beforeEl = s.skl.Get(e)
		if s.beforeEl == nil {
			err = ErrBeforeId
			return
		}
	} else if s.offset > 0 {
		el := s.skl.Front()
		i := 0
		for el != nil {
			i++
			if i == s.offset {
				s.afterId = el.Key().(*entry).id
				s.afterEl = el
				break
			}
			el = el.Next()
		}
		if s.afterEl == nil {
			err = ErrNoRecords
			return
		}
	}

	activeEntries := s.getActiveEntries()
	var activeIds = make([]string, len(activeEntries))
	for i, ae := range activeEntries {
		ae.SetSub(s.id, true, true)
		activeIds[i] = ae.id
	}
	s.diff = newListDiff(activeIds)
	s.compCountBefore.subId = s.id
	s.compCountBefore.prevCount, s.compCountBefore.nextCount = s.counters()
	s.compCountBefore.total = s.skl.Len()

	if s.ds != nil && !s.disableDep {
		s.depKeys = s.ds.depKeys(s.spaceId, s.keys)
		if len(s.depKeys) > 0 || len(s.forceSubIds) > 0 {
			s.depSub = s.ds.makeSubscriptionByEntries(s.id+"/dep", s.spaceId, entries, activeEntries, s.keys, s.depKeys, s.forceSubIds)
		}
	}
	return nil
}

func (s *sortedSub) onChange(ctx *opCtx) {
	var changed bool
	for _, e := range ctx.entries {
		if !s.onEntryChange(ctx, e) {
			changed = true
		}
	}
	if !changed || !s.started {
		return
	}
	defer s.diff.reset()
	s.activeEntriesBuf = s.activeEntriesBuf[:0]
	if s.iterateActive(func(e *entry) {
		s.diff.fillAfter(e.id)
		if s.depSub != nil {
			s.activeEntriesBuf = append(s.activeEntriesBuf, e)
		}
	}) {
		s.diff.reverse()
	}

	s.compCountAfter.subId = s.id
	s.compCountAfter.prevCount, s.compCountAfter.nextCount = s.counters()
	s.compCountAfter.total = s.skl.Len()

	if s.compCountAfter != s.compCountBefore {
		ctx.counters = append(ctx.counters, s.compCountAfter)
		s.compCountBefore = s.compCountAfter
	}

	wasAddOrRemove, added, removed := s.diff.diff(ctx, s.id, s.keys)
	s.ds.depEntriesByEntries(ctx, added)

	hasChanges := false
	for _, e := range ctx.entries {
		if _, ok := s.diff.afterIdsM[e.id]; ok {
			e.SetSub(s.id, true, false)
			ctx.change = append(ctx.change, opChange{
				id:    e.id,
				subId: s.id,
				keys:  s.keys,
			})
			hasChanges = true
		}
	}

	for _, id := range removed {
		if e := s.cache.Get(id); e != nil {
			e.SetSub(s.id, false, false)
		}
	}

	if (wasAddOrRemove || hasChanges) && s.depSub != nil {
		s.ds.refillSubscription(ctx, s.depSub, s.activeEntriesBuf, s.depKeys)
	}

	if s.parent != nil {
		parentEntries, err := queryEntries(s.objectStore, &database.Filters{FilterObj: s.parent.filter})
		if err != nil {
			panic(err)
		}

		var idsForParentFilter []string
		s.iterateActive(func(e *entry) {
			idsForParentFilter = append(idsForParentFilter, e.id)
		})
		s.parentFilter.IDs = idsForParentFilter

		ctx.entries = append(ctx.entries, parentEntries...)
		s.parent.onChange(ctx)
	}
}

func (s *sortedSub) onEntryChange(ctx *opCtx, e *entry) (noChange bool) {
	newInSet := true
	if s.filter != nil {
		newInSet = s.filter.FilterObject(e.data)
	}

	// Accumulate all objects observed before subscription is started
	if !s.started {
		if newInSet {
			s.entriesBeforeStarted = append(s.entriesBeforeStarted, e)
		}
		return true
	}

	curr := s.cache.Get(e.id)
	curInSet := curr != nil
	// nothing
	if !curInSet && !newInSet {
		return true
	}
	// remove
	if curInSet && !newInSet {
		s.skl.Remove(curr)
		e.RemoveSubId(s.id)
		return
	}
	// add
	if !curInSet && newInSet {
		s.skl.Set(e, nil)
		e.SetSub(s.id, false, false)
		return
	}
	// change
	if curInSet && newInSet {
		s.skl.Remove(curr)
		s.skl.Set(e, nil)
		e.SetSub(s.id, false, false)
		return
	}
	panic("subscription: check algo")
}

func (s *sortedSub) counters() (prev, next int) {
	if s.beforeEl == nil && s.afterEl == nil && s.limit <= 0 {
		// no pagination - no counters
		return 0, 0
	}
	getStartEl := func() *skiplist.Element {
		if s.afterEl != nil {
			return s.afterEl
		} else {
			return s.beforeEl
		}
	}
	el := getStartEl()
	for el != nil {
		next++
		el = el.Next()
	}
	el = getStartEl()
	for el != nil {
		prev++
		el = el.Prev()
	}
	if s.afterEl != nil {
		if s.limit > 0 {
			next--
			next -= s.limit
			if next < 0 {
				next = 0
			}
		} else {
			next = 0
		}
	} else if s.beforeEl != nil {
		if s.limit > 0 {
			prev--
			prev -= s.limit
			if prev < 0 {
				prev = 0
			}
		} else {
			prev = 0
		}
	} else if s.limit > 0 {
		next = s.skl.Len() - s.limit
		if next < 0 {
			next = 0
		}
	}
	return
}

func (s *sortedSub) getActiveRecords() (res []*domain.Details) {
	reverse := s.iterateActive(func(e *entry) {
		res = append(res, e.data.CopyOnlyKeys(s.keys...))
	})
	if reverse {
		for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
			res[i], res[j] = res[j], res[i]
		}
	}
	return
}

func (s *sortedSub) getActiveEntries() (entries []*entry) {
	s.activeEntriesBuf = s.activeEntriesBuf[:0]
	s.iterateActive(func(e *entry) {
		s.activeEntriesBuf = append(s.activeEntriesBuf, e)
	})
	return s.activeEntriesBuf
}

func (s *sortedSub) iterateActive(f func(e *entry)) (reverse bool) {
	if s.beforeEl != nil {
		var el = s.beforeEl.Prev()
		var i int
		for el != nil {
			f(el.Key().(*entry))
			i++
			if s.limit > 0 && i >= s.limit {
				break
			}
			el = el.Prev()
		}
		reverse = true
	} else {
		var el = s.skl.Front()
		if s.afterEl != nil {
			el = s.afterEl.Next()
		}
		var i int
		for el != nil {
			f(el.Key().(*entry))
			i++
			if s.limit > 0 && i >= s.limit {
				break
			}
			el = el.Next()
		}
	}
	return
}

// Compare implements sliplist.Comparable
func (s *sortedSub) Compare(lhs, rhs interface{}) (comp int) {
	le := lhs.(*entry)
	re := rhs.(*entry)
	// we need always identify records by id
	if le.id == re.id {
		return 0
	}
	if s.order != nil {
		comp = s.order.Compare(le.data, re.data)
	}
	// when order isn't set or equal - sort by id
	if comp == 0 {
		if le.id > re.id {
			return 1
		} else {
			return -1
		}
	}
	return comp
}

func (s *sortedSub) CalcScore(key interface{}) float64 {
	return 0
}

func (s *sortedSub) hasDep() bool {
	return s.depSub != nil
}

func (s *sortedSub) getDep() subscription {
	return s.depSub
}

func (s *sortedSub) close() {
	if !s.started {
		return
	}
	el := s.skl.Front()
	for el != nil {
		s.cache.RemoveSubId(el.Key().(*entry).id, s.id)
		el = el.Next()
	}
	if s.depSub != nil {
		s.depSub.close()
	}
	for _, child := range s.nested {
		child.close()
	}
}
