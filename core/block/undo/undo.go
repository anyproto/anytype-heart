package undo

import (
	"errors"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

type RelationLinks struct {
	Before, After []*model.RelationLink
}

type ObjectType struct {
	Before, After []string
}

type CarriageInfo struct {
	CarriageBlockID    string
	RangeFrom, RangeTo int32
}

type Action struct {
	Add           []simple.Block
	Change        []Change
	Remove        []simple.Block
	Details       *Details
	RelationLinks *RelationLinks
	Group         string
	ObjectTypes   *ObjectType
	CarriageInfo  CarriageInfo
}

func (a Action) IsEmpty() bool {
	return len(a.Add)+len(a.Change)+len(a.Remove) == 0 && a.Details == nil && a.ObjectTypes == nil && a.RelationLinks == nil
}

func (a Action) Merge(b Action) (result Action) {
	var changedIds []string
	for _, changeB := range b.Change {
		idB := changeB.After.Model().Id
		found := false
		for i, addA := range a.Add {
			idA := addA.Model().Id
			if idA == idB {
				a.Add[i] = changeB.After
				found = true
				break
			}
		}
		if !found {
			for i, changeA := range a.Change {
				idA := changeA.After.Model().Id
				if idA == idB {
					a.Change[i].After = changeB.After
					found = true
					break
				}
			}
		}
		if !found {
			a.Change = append(a.Change, changeB)
		}
		changedIds = append(changedIds, idB)
	}
	return a
}

type History interface {
	Add(a Action)
	Len() int
	Previous() (Action, error)
	Next() (Action, error)
	Reset()
	Counters() (undo int32, redo int32)
	SetCarriageInfo(info CarriageInfo) error
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
	if a.IsEmpty() {
		return
	}
	if a.Group != "" {
		if h.applyGroup(a) {
			return
		}
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

func (h *history) Counters() (undo int32, redo int32) {
	return int32(h.pointer), int32(len(h.actions) - h.pointer)
}

func (h *history) SetCarriageInfo(info CarriageInfo) error {
	if h.pointer > 0 {
		h.actions[h.pointer-1].CarriageInfo = info
		return nil
	}
	return ErrNoHistory
}

func (h *history) applyGroup(b Action) (ok bool) {
	for i, a := range h.actions {
		if a.Group == b.Group {
			h.actions[i] = a.Merge(b)
			return true
		}
	}
	return false
}
