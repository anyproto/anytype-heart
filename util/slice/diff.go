package slice

import (
	"fmt"

	"github.com/mb0/diff"
)

func StringIdentity[T ~string](x T) string {
	return string(x)
}

func Equal[T comparable](a, b T) bool {
	return a == b
}

type Change[T any] struct {
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

func MakeChangeAdd[T any](items []T, afterID string) Change[T] {
	return Change[T]{
		changeAdd: &ChangeAdd[T]{items, afterID},
	}
}

func MakeChangeRemove[T any](ids []string) Change[T] {
	return Change[T]{
		changeRemove: &ChangeRemove{ids},
	}
}

func MakeChangeMove[T any](ids []string, afterID string) Change[T] {
	return Change[T]{
		changeMove: &ChangeMove{ids, afterID},
	}
}

func MakeChangeReplace[T any](item T, id string) Change[T] {
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

type ChangeAdd[T any] struct {
	Items   []T
	AfterId string
}

func (c ChangeAdd[T]) String() string {
	return fmt.Sprintf("add %v after %s", c.Items, c.AfterId)
}

type ChangeMove struct {
	IDs     []string
	AfterID string
}

func (c ChangeMove) String() string {
	return fmt.Sprintf("move %v after %s", c.IDs, c.AfterID)
}

type ChangeRemove struct {
	IDs []string
}

func (c ChangeRemove) String() string {
	return fmt.Sprintf("remove %v", c.IDs)
}

type ChangeReplace[T any] struct {
	Item T
	ID   string
}

func (c ChangeReplace[T]) String() string {
	return fmt.Sprintf("replace %v after %s", c.Item, c.ID)
}

type MixedInput[T any] struct {
	A     []T
	B     []T
	getID func(T) string
}

func (m *MixedInput[T]) Equal(a, b int) bool {
	return m.getID(m.A[a]) == m.getID(m.B[b])
}

func Diff[T any](origin, changed []T, getID func(T) string, equal func(T, T) bool) []Change[T] {
	m := &MixedInput[T]{
		origin,
		changed,
		getID,
	}

	var result []Change[T]

	changes := diff.Diff(len(m.A), len(m.B), m)
	delMap := make(map[string]T)

	changedMap := make(map[string]T)
	for _, c := range changed {
		changedMap[getID(c)] = c
	}
	for _, c := range origin {
		v, ok := changedMap[getID(c)]
		if !ok {
			continue
		}
		if !equal(c, v) {
			result = append(result, MakeChangeReplace[T](v, getID(c)))
		}
	}

	for _, c := range changes {
		if c.Del <= 0 {
			continue
		}
		for _, id := range m.A[c.A : c.A+c.Del] {
			delMap[getID(id)] = id
		}
	}

	for _, c := range changes {
		if c.Ins <= 0 {
			continue
		}

		inserts := m.B[c.B : c.B+c.Ins]
		var afterID string
		if c.A > 0 {
			afterID = getID(m.A[c.A-1])
		}
		var oneCh Change[T]
		for _, it := range inserts {
			id := getID(it)
			if _, ok := delMap[id]; ok { // move
				mv := oneCh.Move()
				if mv == nil {
					if oneCh.Len() > 0 {
						result = append(result, oneCh)
					}
					oneCh = MakeChangeMove[T](nil, afterID)
					mv = oneCh.Move()
				}
				mv.IDs = append(mv.IDs, getID(it))
				delete(delMap, id)
			} else { // insert new
				add := oneCh.Add()
				if add == nil {
					if oneCh.Len() > 0 {
						result = append(result, oneCh)
					}
					oneCh = MakeChangeAdd[T](nil, afterID)
					add = oneCh.Add()
				}
				add.Items = append(add.Items, it)
			}
			afterID = id
		}

		if oneCh.Len() > 0 {
			result = append(result, oneCh)
		}

	}

	if len(delMap) > 0 { // remove
		delIDs := make([]string, 0, len(delMap))
		for id := range delMap {
			delIDs = append(delIDs, id)
		}
		result = append(result, MakeChangeRemove[T](delIDs))
	}
	return result
}

func findPos[T any](s []T, getID func(T) string, id string) int {
	for i, sv := range s {
		if getID(sv) == id {
			return i
		}
	}
	return -1
}

func ApplyChanges[T any](origin []T, changes []Change[T], getID func(T) string) []T {
	res := make([]T, len(origin))
	copy(res, origin)

	itemsMap := make(map[string]T, len(origin))
	for _, it := range origin {
		itemsMap[getID(it)] = it
	}

	for _, ch := range changes {
		if add := ch.Add(); add != nil {
			pos := findPos(res, getID, add.AfterId)
			res = Insert(res, pos+1, add.Items...)
		}

		if move := ch.Move(); move != nil {
			res = FilterMut(res, func(id T) bool {
				return FindPos(move.IDs, getID(id)) < 0
			})

			pos := findPos(res, getID, move.AfterID)
			items := make([]T, 0, len(move.IDs))
			for _, id := range move.IDs {
				v, ok := itemsMap[id]
				if !ok {
					continue
				}
				items = append(items, v)
			}
			res = Insert(res, pos+1, items...)
		}

		if rm := ch.Remove(); rm != nil {
			res = FilterMut(res, func(id T) bool {
				return FindPos(rm.IDs, getID(id)) < 0
			})
		}

		if replace := ch.Replace(); replace != nil {
			itemsMap[replace.ID] = replace.Item
			pos := findPos(res, getID, replace.ID)
			if pos >= 0 && pos < len(res) {
				res[pos] = replace.Item
			}
		}
	}

	return res
}
