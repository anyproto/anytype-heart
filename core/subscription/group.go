package subscription

//import (
//	"github.com/gogo/protobuf/types"
//)
//
//func (s *service) newGroupSub(id string, groups [][]string) *groupSub {
//	sub := &groupSub{
//		id:    id,
//		cache: s.cache,
//	}
//	return sub
//}
//
//type groupSub struct {
//	id  string
//	set map[string]struct{}
//
//	activeEntriesBuf []*entry
//
//	cache *cache
//
//	groups map[string][]string
//}
//
//func (s *groupSub) init(entries []*entry) (err error) {
//	s.set = make(map[string]struct{})
//	for _, e := range entries {
//		e = s.cache.GetOrSet(e)
//		s.set[e.id] = struct{}{}
//		e.SetSub(s.id, true)
//	}
//	return
//}
//
//func (s *groupSub) refill(ctx *opCtx, entries []*entry) {
//	var newSet = make(map[string]struct{})
//	for _, e := range entries {
//		if _, inSet := s.set[e.id]; inSet {
//			ctx.change = append(ctx.change, opChange{
//				id:    e.id,
//				subId: s.id,
//				keys:  s.keys,
//			})
//		} else {
//			ctx.position = append(ctx.position, opPosition{
//				id:    e.id,
//				subId: s.id,
//				keys:  s.keys,
//				isAdd: true,
//			})
//		}
//		newSet[e.id] = struct{}{}
//		e.SetSub(s.id, true)
//	}
//	for oldId := range s.set {
//		if _, inSet := newSet[oldId]; !inSet {
//			ctx.remove = append(ctx.remove, opRemove{
//				id:    oldId,
//				subId: s.id,
//			})
//			s.cache.RemoveSubId(oldId, s.id)
//		}
//	}
//	s.set = newSet
//}
//
//func (s *groupSub) counters() (prev, next int) {
//	return 0, 0
//}
//
//func (s *groupSub) onChange(ctx *opCtx) {
//	for _, e := range ctx.entries {
//		if _, inSet := s.set[e.id]; inSet {
//			ctx.change = append(ctx.change, opChange{
//				id:    e.id,
//				subId: s.id,
//				keys:  s.keys,
//			})
//			e.SetSub(s.id, true)
//		}
//	}
//}
//
//func (s *groupSub) getActiveRecords() (res []*types.Struct) {
//	return
//}
//
//func (s *groupSub) hasDep() bool {
//	return false
//}
//
//func (s *groupSub) close() {
//	for id := range s.set {
//		s.cache.RemoveSubId(id, s.id)
//	}
//	return
//}
