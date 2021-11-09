package subscription

import (
	"errors"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
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
	afterInSet := true
	if s.filter != nil {
		afterInSet = s.filter.FilterObject(e)
	}
	inSet, inActive := s.lookup(e.id)

	if inSet && !afterInSet {
		s.remove(ctx, inActive)
		return true
	}
	if !inSet && afterInSet {
		s.add(ctx)
		return true
	}

	return
}

func (s *subscription) onRemove(events []*pb.EventMessage, id string) []*pb.EventMessage {
	return events
}

func (s *subscription) remove(ctx *opCtx, active bool) {

}

func (s *subscription) add(ctx *opCtx) {

}

func (s *subscription) lookup(id string) (inSet, inActive bool) {
	e := s.cache.pick(id)
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
