package slice

import (
	"github.com/mb0/diff"
)

type DiffOperation int

const (
	OperationNone DiffOperation = iota
	OperationAdd
	OperationMove
	OperationRemove
	OperationReplace
)

type Change[T IDGetter] struct {
	Op DiffOperation
	// TODO rename
	Items   []T
	AfterId string
}

type IDGetter interface {
	GetId() string
}

type MixedInput[T IDGetter] struct {
	A []T
	B []T
}

func (m *MixedInput[T]) Equal(a, b int) bool {
	return m.A[a].GetId() == m.B[b].GetId()
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

func Diff[T IDGetter](origin, changed []T) []Change[T] {
	m := &MixedInput[T]{
		origin,
		changed,
	}

	var result []Change[T]

	changes := diff.Diff(len(m.A), len(m.B), m)
	delMap := make(map[string]T)
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
					if oneCh.Op != OperationMove {
						if len(oneCh.Items) > 0 {
							result = append(result, oneCh)
						}
						oneCh = Change[T]{Op: OperationMove, AfterId: afterId}
					}
					oneCh.Items = append(oneCh.Items, it)
					delete(delMap, id)
				} else { // insert new
					if oneCh.Op != OperationAdd {
						if len(oneCh.Items) > 0 {
							result = append(result, oneCh)
						}
						oneCh = Change[T]{Op: OperationAdd, AfterId: afterId}
					}
					oneCh.Items = append(oneCh.Items, it)
				}
				afterId = id
			}

			if len(oneCh.Items) > 0 {
				result = append(result, oneCh)
			}
		}
	}

	if len(delMap) > 0 { // remove
		delIds := make([]T, 0, len(delMap))
		for _, it := range delMap {
			delIds = append(delIds, it)
		}
		// TODO maybe just use ID wrapper, don't store WHOLE items
		result = append(result, Change[T]{Op: OperationRemove, Items: delIds})
	}

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

	for _, ch := range changes {
		switch ch.Op {
		case OperationAdd:
			pos := -1
			if ch.AfterId != "" {
				pos = findPos(res, ch.AfterId)
				if pos < 0 {
					continue
				}
			}
			res = Insert(res, pos+1, ch.Items...)
		case OperationMove:
			withoutMoved := Filter(res, func(id T) bool {
				return findPos(ch.Items, id.GetId()) < 0
			})
			pos := -1
			if ch.AfterId != "" {
				pos = findPos(withoutMoved, ch.AfterId)
				if pos < 0 {
					continue
				}
			}
			res = Insert(withoutMoved, pos+1, ch.Items...)
		case OperationRemove:
			res = Filter(res, func(id T) bool {
				return findPos(ch.Items, id.GetId()) < 0
			})
		case OperationReplace:
			res = ch.Items
		}
	}

	return res
}
