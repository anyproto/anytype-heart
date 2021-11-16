package subscription

import (
	"errors"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/gogo/protobuf/types"
	"github.com/huandu/skiplist"
)

var (
	ErrAfterId  = errors.New("after id not in set")
	ErrBeforeId = errors.New("before id not in set")
)

func (s *service) newSubscription(id string, keys []string, filter filter.Filter, order filter.Order) *subscription {
	sub := &subscription{
		id:     id,
		keys:   keys,
		filter: filter,
		order:  order,
		cache:  s.cache,
	}
	return sub
}

type subscription struct {
	id     string
	keys   []string
	filter filter.Filter
	order  filter.Order

	afterId, beforeId string
	limit             int

	skl               *skiplist.SkipList
	afterEl, beforeEl *skiplist.Element

	cache *cache
}

func (s *subscription) fill(entries []*entry) (err error) {
	s.skl = skiplist.New(s)

	for _, e := range entries {
		s.cache.set(e)
		s.skl.Set(s.cache.get(e.id), nil)
	}
	if s.afterId != "" {
		e := s.cache.pick(s.afterId)
		if e == nil {
			return ErrAfterId
		}
		s.afterEl = s.skl.Get(e)
	} else if s.beforeId != "" {
		e := s.cache.pick(s.beforeId)
		if e == nil {
			return ErrBeforeId
		}
		s.beforeEl = s.skl.Get(e)
	}
	return nil
}

func (s *subscription) onChangeBatch(ctx *opCtx, entries ...*entry) {
	var countersChanged bool
	for _, e := range entries {
		if s.onChange(ctx, e) {
			countersChanged = true
		}
	}
	if countersChanged {
		prev, next := s.counters()
		ctx.counters = append(ctx.counters, opCounter{
			subId:     s.id,
			total:     s.skl.Len(),
			prevCount: prev,
			nextCount: next,
		})
	}
}

func (s *subscription) onChange(ctx *opCtx, e *entry) (countersChange bool) {
	newInSet := true
	if s.filter != nil {
		newInSet = s.filter.FilterObject(e)
	}
	curInSet, currInActive := s.lookup(s.cache.pick(e.id))
	if !curInSet && !newInSet {
		return false
	}
	if curInSet && !newInSet {
		if currInActive {
			s.removeActive(ctx, e)
		} else {
			s.removeNonActive(e.id)
		}
		return true
	}
	if !curInSet && newInSet {
		return s.add(ctx, e)
	}
	if curInSet && newInSet {
		return s.change(ctx, e, currInActive)
	}
	panic("subscription: check algo")
}

func (s *subscription) removeNonActive(id string) {
	e := s.cache.pick(id)
	if s.afterEl != nil {
		if comp := s.Compare(s.afterEl.Key(), s.skl.Get(e).Key()); comp <= 0 {
			if comp == 0 {
				s.afterEl = s.afterEl.Prev()
				if s.afterEl != nil {
					s.afterId = s.afterEl.Key().(*entry).id
				}
			}
		}
	} else if s.beforeEl != nil {
		if comp := s.Compare(s.beforeEl.Key(), s.skl.Get(e).Key()); comp >= 0 {
			if comp == 0 {
				s.beforeEl = s.beforeEl.Next()
				if s.beforeEl != nil {
					s.beforeId = s.beforeEl.Key().(*entry).id
				}
			}
		}
	}
	s.skl.Remove(e)
	s.cache.release(e.id)
}

func (s *subscription) removeActive(ctx *opCtx, e *entry) {
	s.skl.Remove(s.cache.pick(e.id))
	s.cache.release(e.id)
	ctx.remove = append(ctx.remove, opRemove{
		id:    e.id,
		subId: s.id,
	})
	s.alignAdd(ctx)
}

func (s *subscription) add(ctx *opCtx, e *entry) (countersChange bool) {
	s.skl.Set(e, nil)
	s.cache.get(e.id)
	_, inActive := s.lookup(e)
	if inActive {
		var afterId string
		if prev := s.skl.Get(e).Prev(); prev != nil {
			afterId = prev.Key().(*entry).id
		}
		ctx.add = append(ctx.add, opChange{
			id:      e.id,
			subId:   s.id,
			keys:    s.keys,
			afterId: afterId,
		})
		s.alignRemove(ctx)
		return false
	}
	return true
}

func (s *subscription) change(ctx *opCtx, e *entry, currInActive bool) (countersChange bool) {
	var currAfterId string
	if currInActive {
		if prev := s.skl.Get(s.cache.pick(e.id)).Prev(); prev != nil {
			currAfterId = prev.Key().(*entry).id
		}
	}
	s.skl.Remove(s.cache.pick(e.id))
	s.skl.Set(e, nil)
	_, newInActive := s.lookup(e)
	if newInActive {
		var newAfterId string
		if prev := s.skl.Get(e).Prev(); prev != nil {
			newAfterId = prev.Key().(*entry).id
		}
		if currAfterId != newAfterId {
			ctx.position = append(ctx.position, opPosition{
				id:      e.id,
				subId:   s.id,
				afterId: newAfterId,
			})
		}
		if !currInActive {
			countersChange = true
		}
		ctx.change = append(ctx.change, opChange{
			id:    e.id,
			subId: s.id,
			keys:  s.keys,
		})
	} else {
		if currInActive {
			ctx.remove = append(ctx.remove, opRemove{
				id:    e.id,
				subId: s.id,
			})
			s.alignAdd(ctx)
		}
		countersChange = true
	}
	return
}

func (s *subscription) alignAdd(ctx *opCtx) {
	if s.limit > 0 {
		if s.beforeEl != nil {
			ctx.add = append(ctx.add, opChange{
				id:    s.beforeEl.Key().(*entry).id,
				subId: s.id,
				keys:  s.keys,
			})
			s.beforeEl = s.beforeEl.Next()
			if s.beforeEl != nil {
				s.beforeId = s.beforeEl.Key().(*entry).id
			}
		} else {
			var i int
			var next = s.afterEl
			if next == nil {
				next = s.skl.Front()
			} else {
				next = next.Next()
			}
			for next != nil {
				i++
				if i == s.limit {
					break
				}
				next = next.Next()
			}
			if next != nil {
				afterId := ""
				prev := next.Prev()
				if prev != nil {
					afterId = prev.Key().(*entry).id
				}
				ctx.add = append(ctx.add, opChange{
					id:      next.Key().(*entry).id,
					afterId: afterId,
					subId:   s.id,
					keys:    s.keys,
				})
			}
		}
	}
}

func (s *subscription) alignRemove(ctx *opCtx) {
	if s.limit > 0 {
		if s.beforeEl != nil {
			ctx.remove = append(ctx.remove, opRemove{
				id:    s.beforeEl.Key().(*entry).id,
				subId: s.id,
			})
			s.beforeEl = s.beforeEl.Prev()
			if s.beforeEl != nil {
				s.beforeId = s.beforeEl.Key().(*entry).id
			}
		} else {
			var i int
			var next = s.afterEl
			if next == nil {
				next = s.skl.Front()
			} else {
				next = next.Next()
			}
			for next != nil {
				if i == s.limit {
					break
				}
				next = next.Next()
				i++
			}
			if next != nil {
				ctx.remove = append(ctx.remove, opRemove{
					id:    next.Key().(*entry).id,
					subId: s.id,
				})
			}
		}
	}
}

func (s *subscription) lookup(e *entry) (inSet, inActive bool) {
	if e == nil {
		return
	}
	el := s.skl.Get(e)
	if el == nil {
		return
	}
	inSet = true
	var (
		startEl  *skiplist.Element
		backward bool
	)
	if s.afterEl != nil {
		startEl = s.afterEl
	} else if s.beforeEl != nil {
		startEl = s.beforeEl
		backward = true
	}

	if startEl != nil {
		comp := s.Compare(startEl.Key(), e)
		if comp == 0 {
			return
		}
		if (comp < 0 && backward) || (comp > 0 && !backward) {
			return
		}
		if s.limit > 0 {
			if s.inDistance(startEl, e.id, s.limit, backward) {
				inActive = true
			}
		} else {
			inActive = true
		}
	} else if s.limit > 0 {
		if s.inDistance(s.skl.Front(), e.id, s.limit-1, false) {
			inActive = true
		}
	} else {
		inActive = true
	}
	return
}

func (s *subscription) counters() (prev, next int) {
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

func (s *subscription) inDistance(el *skiplist.Element, id string, distance int, backward bool) bool {
	var i int
	for el != nil {
		if el.Key().(*entry).id == id {
			return true
		}
		i++
		if i > distance {
			return false
		}
		if backward {
			el = el.Prev()
		} else {
			el = el.Next()
		}
	}
	return false
}

func (s *subscription) getActiveRecords() (res []*types.Struct) {
	if s.beforeEl != nil {
		var el = s.beforeEl.Prev()
		for el != nil {
			res = append(res, el.Key().(*entry).data)
			if s.limit > 0 && len(res) >= s.limit {
				break
			}
			el = el.Prev()
		}
		for i, j := 0, len(res)-1; i < j; i, j = i+1, j-1 {
			res[i], res[j] = res[j], res[i]
		}
	} else {
		var el = s.skl.Front()
		if s.afterEl != nil {
			el = s.afterEl.Next()
		}
		for el != nil {
			res = append(res, el.Key().(*entry).data)
			if s.limit > 0 && len(res) >= s.limit {
				break
			}
			el = el.Next()
		}
	}
	return
}

// Compare implements sliplist.Comparable
func (s *subscription) Compare(lhs, rhs interface{}) (comp int) {
	le := lhs.(*entry)
	re := rhs.(*entry)
	// we need always identify records by id
	if le.id == re.id {
		return 0
	}
	if s.order != nil {
		comp = s.order.Compare(le, re)
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

func (s *subscription) CalcScore(key interface{}) float64 {
	return 0
}

func (s *subscription) close() {
	el := s.skl.Front()
	for el != nil {
		s.cache.release(el.Key().(*entry).id)
		el = el.Next()
	}
}

type entrySorter struct {
	entries []*entry
	s       *subscription
}

func (e entrySorter) Len() int {
	return len(e.entries)
}

func (e entrySorter) Less(i, j int) bool {
	return e.s.Compare(e.entries[i], e.entries[j]) == -1
}

func (e entrySorter) Swap(i, j int) {
	e.entries[i], e.entries[j] = e.entries[j], e.entries[i]
}
