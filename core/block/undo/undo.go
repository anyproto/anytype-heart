package undo

import (
	"errors"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

const (
	defaultLimit = 300
)

var (
	ErrNoHistory = errors.New("no history")
)

type Change struct {
	Before, After simple.Block
}

type Details struct {
	Before, After *types.Struct
}

type Action struct {
	Add      []simple.Block
	Change   []Change
	Remove   []simple.Block
	Details  *Details
	groupIds []string
}

func (a Action) IsEmpty() bool {
	return len(a.Add)+len(a.Change)+len(a.Remove) == 0 && a.Details == nil
}

func (a *Action) HandleGroupBlocks(apply func(groupId string, b simple.Block) bool) {
	filteredAdd := a.Add[:0]
	for _, add := range a.Add {
		if gr, ok := add.(simple.UndoGroup); ok {
			if groupId := gr.UndoGroupId(); groupId != "" {
				if apply(groupId, add) {
					continue
				} else {
					a.groupIds = append(a.groupIds, groupId)
				}
			}
		}
		filteredAdd = append(filteredAdd, add)
	}
	a.Add = filteredAdd
	filteredChange := a.Change[:0]
	for _, change := range a.Change {
		if gr, ok := change.After.(simple.UndoGroup); ok {
			if groupId := gr.UndoGroupId(); groupId != "" {
				if apply(groupId, change.After) {
					continue
				} else {
					a.groupIds = append(a.groupIds, groupId)
				}
			}
		}
		filteredChange = append(filteredChange, change)
	}
	a.Change = filteredChange
}

type History interface {
	Add(a Action)
	Len() int
	Previous() (Action, error)
	Next() (Action, error)
	Reset()
}

func NewHistory(limit int) History {
	if limit <= 0 {
		limit = defaultLimit
	}
	return &history{limit: limit}
}

type history struct {
	limit   int
	actions []Action
	pointer int
}

func (h *history) Add(a Action) {
	act := &a
	act.HandleGroupBlocks(h.applyToGroup)
	if act.IsEmpty() {
		return
	}
	if len(h.actions) != h.pointer {
		h.actions = h.actions[:h.pointer]
	}
	h.actions = append(h.actions, a)
	h.pointer = len(h.actions)
	if h.pointer > h.limit {
		h.actions[0] = Action{}
		h.actions = h.actions[1:]
		h.pointer--
	}
}

func (h *history) Len() int {
	return h.pointer
}

func (h *history) Previous() (Action, error) {
	if h.pointer > 0 {
		h.pointer--
		return h.actions[h.pointer], nil
	}
	return Action{}, ErrNoHistory
}

func (h *history) Next() (Action, error) {
	if h.pointer < len(h.actions) {
		action := h.actions[h.pointer]
		h.pointer++
		return action, nil
	}
	return Action{}, ErrNoHistory
}

func (h *history) Reset() {
	h.pointer = 0
	h.actions = h.actions[:0]
}

func (h *history) applyToGroup(gId string, b simple.Block) (ok bool) {
	for i, a := range h.actions {
		if slice.FindPos(a.groupIds, gId) != -1 {
			for ai, add := range a.Add {
				if gr, ok := add.(simple.UndoGroup); ok {
					if groupId := gr.UndoGroupId(); groupId == gId {
						a.Add[ai] = b
						h.actions[i] = a
						return true
					}
				}
			}
			for ci, change := range a.Change {
				if gr, ok := change.After.(simple.UndoGroup); ok {
					if groupId := gr.UndoGroupId(); groupId == gId {
						change.After = b
						a.Change[ci] = change
						h.actions[i] = a
						return true
					}
				}
			}
		}
	}
	return false
}
