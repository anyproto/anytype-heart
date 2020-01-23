package history

import (
	"errors"

	"github.com/anytypeio/go-anytype-library/pb/model"
)

const (
	defaultLimit = 300
)

var (
	ErrNoHistory = errors.New("no history")
)

type Action struct {
	Add    []model.Block
	Change []model.Block
	Remove []model.Block
}

func (a Action) IsEmpty() bool {
	return len(a.Add)+len(a.Change)+len(a.Remove) == 0
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
