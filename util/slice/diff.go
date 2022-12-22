package slice

import (
	"fmt"

	"github.com/mb0/diff"
)

// type DiffOperation int
//
// const (
// 	OperationNone DiffOperation = iota
// 	OperationAdd
// 	OperationMove
// 	OperationRemove
// 	OperationReplace
// )

// type Change[T IDGetter] struct {
// 	Op DiffOperation
// 	Items   []T
// 	AfterId string
// }

type Change[T IDGetter] struct {
	changeAdd     *ChangeAdd[T]
	changeRemove  *ChangeRemove
	changeMove    *ChangeMove
	changeReplace *ChangeReplace[T]
}

func (c Change[T]) String() string {
	switch {
	case c.changeAdd != nil:
		return c.changeAdd.String()
	case c.changeRemove != nil:
		return c.changeRemove.String()
	case c.changeMove != nil:
		return c.changeMove.String()
	case c.changeReplace != nil:
		return c.changeReplace.String()
	}
	return ""
}

func MakeChangeAdd[T IDGetter](items []T, afterId string) Change[T] {
	return Change[T]{
		changeAdd: &ChangeAdd[T]{items, afterId},
	}
}

func MakeChangeRemove[T IDGetter](ids []string) Change[T] {
	return Change[T]{
		changeRemove: &ChangeRemove{ids},
	}
}

func MakeChangeMove[T IDGetter](ids []string, afterID string) Change[T] {
	return Change[T]{
		changeMove: &ChangeMove{ids, afterID},
	}
}

func MakeChangeReplace[T IDGetter](item T, id string) Change[T] {
	return Change[T]{
		changeReplace: &ChangeReplace[T]{item, id},
	}
}

func (c Change[T]) Len() int {
	if c.changeAdd != nil {
		return len(c.changeAdd.Items)
	}
	if c.changeRemove != nil {
		return len(c.changeRemove.IDs)
	}
	if c.changeMove != nil {
		return len(c.changeMove.IDs)
	}
	if c.changeReplace != nil {
		return 1
	}
	return 0
}

func (c *Change[T]) Match(add func(*ChangeAdd[T]), remove func(*ChangeRemove), move func(*ChangeMove), replace func(*ChangeReplace[T])) {
	if c.changeAdd != nil {
		add(c.changeAdd)
	}
	if c.changeRemove != nil {
		remove(c.changeRemove)
	}
	if c.changeMove != nil {
		move(c.changeMove)
	}
	if c.changeReplace != nil {
		replace(c.changeReplace)
	}
}

func (c *Change[T]) Add() *ChangeAdd[T] {
	return c.changeAdd
}

func (c *Change[T]) Remove() *ChangeRemove {
	return c.changeRemove
}

func (c *Change[T]) Move() *ChangeMove {
	return c.changeMove
}

func (c *Change[T]) Replace() *ChangeReplace[T] {
	return c.changeReplace
}

type ChangeAdd[T IDGetter] struct {
	Items   []T
	AfterId string
}

func (c ChangeAdd[T]) String() string {
	return fmt.Sprintf("add %v after %s", c.Items, c.AfterId)
}

type ChangeMove struct {
	IDs     []string
	AfterId string
}

func (c ChangeMove) String() string {
	return fmt.Sprintf("move %v after %s", c.IDs, c.AfterId)
}

type ChangeRemove struct {
	IDs []string
}

func (c ChangeRemove) String() string {
	return fmt.Sprintf("remove %v", c.IDs)
}

type ChangeReplace[T IDGetter] struct {
	Item T
	ID   string
}

func (c ChangeReplace[T]) String() string {
	return fmt.Sprintf("replace %v after %s", c.Item, c.ID)
}

type IDGetter interface {
	GetId() string
}

type MixedInput[T IDGetter] struct {
	A       []T
	B       []T
	compare func(T, T) bool
}

func (m *MixedInput[T]) Equal(a, b int) bool {
	return m.A[a].GetId() == m.B[b].GetId()
	// return m.compare(m.A[a], m.B[b])
}

type ID string

func (id ID) GetId() string { return string(id) }

func StringsToIDs(ss []string) []ID {
	ids := make([]ID, 0, len(ss))
	for _, s := range ss {
		ids = append(ids, ID(s))
	}
	return ids
}

func IDsToStrings(ids []ID) []string {
	ss := make([]string, 0, len(ids))
	for _, id := range ids {
		ss = append(ss, string(id))
	}
	return ss
}

func CompareID(a, b ID) bool { return a == b }

func Diff[T IDGetter](origin, changed []T, equal func(T, T) bool) []Change[T] {
	m := &MixedInput[T]{
		origin,
		changed,
		equal,
	}

	var result []Change[T]

	changes := diff.Diff(len(m.A), len(m.B), m)
	delMap := make(map[string]T)

	// TODO handle replace
	changedMap := make(map[string]T)
	for _, c := range changed {
		changedMap[c.GetId()] = c
	}
	for _, c := range origin {
		v, ok := changedMap[c.GetId()]
		if !ok {
			continue
		}
		if !equal(c, v) {
			result = append(result, MakeChangeReplace[T](v, c.GetId()))
		}
	}

	for _, c := range changes {
		if c.Del > 0 {
			for _, id := range m.A[c.A : c.A+c.Del] {
				delMap[id.GetId()] = id
			}
		}
	}

	for _, c := range changes {
		if c.Ins > 0 {
			inserts := m.B[c.B : c.B+c.Ins]
			afterId := ""
			if c.A > 0 {
				afterId = m.A[c.A-1].GetId()
			}
			var oneCh Change[T]
			for _, it := range inserts {
				id := it.GetId()
				if _, ok := delMap[id]; ok { // move
					mv := oneCh.Move()
					if mv == nil {
						if oneCh.Len() > 0 {
							result = append(result, oneCh)
						}
						oneCh = MakeChangeMove[T](nil, afterId)
						mv = oneCh.Move()
					}
					mv.IDs = append(mv.IDs, it.GetId())
					delete(delMap, id)
				} else { // insert new
					add := oneCh.Add()
					if add == nil {
						if oneCh.Len() > 0 {
							result = append(result, oneCh)
						}
						oneCh = MakeChangeAdd[T](nil, afterId)
						add = oneCh.Add()
					}
					add.Items = append(add.Items, it)
				}
				afterId = id
			}

			if oneCh.Len() > 0 {
				result = append(result, oneCh)
			}
		}
	}

	if len(delMap) > 0 { // remove
		delIDs := make([]string, 0, len(delMap))
		for id := range delMap {
			delIDs = append(delIDs, id)
		}
		result = append(result, MakeChangeRemove[T](delIDs))
	}
	//
	// originMap := make(map[string]T)
	// for _, it := range origin {
	// 	originMap[it.GetId()] = it
	// }
	// changedMap := make(map[string]T)
	// for _, it := range changed {
	// 	changedMap[it.GetId()] = it
	// }
	//
	// for _, c := range result {
	// 	mv := c.Move()
	// 	if mv == nil {
	// 		continue
	// 	}
	//
	// 	for _, id := range mv.IDs {
	// 		if !equal(originMap[id], changedMap[id]) {
	// 			result = append(result, MakeChangeReplace[T](changedMap[id], id))
	// 		}
	// 	}
	//
	// }

	return result
}

func findPos[T IDGetter](s []T, id string) int {
	for i, sv := range s {
		if sv.GetId() == id {
			return i
		}
	}
	return -1
}

func ApplyChanges[T IDGetter](origin []T, changes []Change[T]) []T {
	res := make([]T, len(origin))
	copy(res, origin)

	itemsMap := make(map[string]T, len(origin))
	for _, it := range origin {
		itemsMap[it.GetId()] = it
	}

	for _, ch := range changes {
		if add := ch.Add(); add != nil {
			pos := -1
			if add.AfterId != "" {
				pos = findPos(res, add.AfterId)
				if pos < 0 {
					continue
				}
			}
			res = Insert(res, pos+1, add.Items...)
		}

		if move := ch.Move(); move != nil {
			withoutMoved := Filter(res, func(id T) bool {
				return FindPos(move.IDs, id.GetId()) < 0
			})
			pos := -1
			if move.AfterId != "" {
				pos = findPos(withoutMoved, move.AfterId)
				if pos < 0 {
					continue
				}
			}

			items := make([]T, 0, len(move.IDs))
			for _, id := range move.IDs {
				items = append(items, itemsMap[id])
			}
			res = Insert(withoutMoved, pos+1, items...)
		}

		if rm := ch.Remove(); rm != nil {
			res = Filter(res, func(id T) bool {
				return FindPos(rm.IDs, id.GetId()) < 0
			})
		}

		if replace := ch.Replace(); replace != nil {
			itemsMap[replace.ID] = replace.Item
			pos := findPos(res, replace.ID)
			if pos >= 0 && pos < len(res) {
				res[pos] = replace.Item
			}
		}
	}

	return res
}
