package slice

import (
	"github.com/mb0/diff"
)

type DiffOperation int

const (
	OperationNone    DiffOperation = iota
	OperationAdd
	OperationMove
	OperationRemove
	OperationReplace
)

type Change struct {
	Op      DiffOperation
	Ids     []string
	AfterId string
}

type MixedInput struct {
	A []string
	B []string
}

func (m *MixedInput) Equal(a, b int) bool {
	return m.A[a] == m.B[b]
}

func Diff(origin, changed []string) []Change {
	m := &MixedInput{
		origin,
		changed,
	}

	var result []Change

	changes := diff.Diff(len(m.A), len(m.B), m)
	delMap := make(map[string]bool)
	for _, c := range changes {
		if c.Del > 0 {
			for _, id := range m.A[c.A:c.A+c.Del] {
				delMap[id] = true
			}
		}
	}

	for _, c := range changes {
		if c.Ins > 0 {
			inserts := m.B[c.B:c.B+c.Ins]
			afterId := ""
			if c.A >  0 {
				afterId = m.A[c.A-1]
			}
			var oneCh Change
			for _, id := range inserts {
				if delMap[id] { // move
					if oneCh.Op != OperationMove {
						if len(oneCh.Ids) > 0 {
							result = append(result, oneCh)
						}
						oneCh = Change{Op: OperationMove, AfterId: afterId}
					}
					oneCh.Ids = append(oneCh.Ids, id)
					delete(delMap, id)
				} else { // insert new
					if oneCh.Op != OperationAdd {
						if len(oneCh.Ids) > 0 {
							result = append(result, oneCh)
						}
						oneCh = Change{Op: OperationAdd, AfterId: afterId}
					}
					oneCh.Ids = append(oneCh.Ids, id)
				}
				afterId = id
			}

			if len(oneCh.Ids) > 0 {
				result = append(result, oneCh)
			}
		}
	}

	if len(delMap) > 0 { // remove
		delIds := make([]string, len(delMap))
		for id := range delMap {
			delIds = append(delIds, id)
		}
		result = append(result, Change{Op: OperationRemove, Ids: delIds})
	}

	return result
}

func ApplyChanges(origin []string, changes []Change) []string {
	result := make([]string, len(origin))
	copy(result, origin)

	for _, ch := range changes {
		switch ch.Op {
		case OperationAdd:
			pos := -1
			if ch.AfterId != "" {
				pos = FindPos(result, ch.AfterId)
				if pos < 0 {
					continue
				}
			}
			result = Insert(result, pos+1, ch.Ids...)
		case OperationMove:
			withoutMoved := Filter(result, func(id string) bool {
				return FindPos(ch.Ids, id) < 0
			})
			pos := -1
			if ch.AfterId != "" {
				pos = FindPos(withoutMoved, ch.AfterId)
				if pos < 0 {
					continue
				}
			}
			result = Insert(withoutMoved, pos+1, ch.Ids...)
		case OperationRemove:
			result = Filter(result, func(id string) bool{
				return FindPos(ch.Ids, id) < 0
			})
		case OperationReplace:
			result = ch.Ids
		}
	}

	return result
}
