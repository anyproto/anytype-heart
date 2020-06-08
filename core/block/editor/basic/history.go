package basic

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
)

type IHistory interface {
	Undo(*state.Context) (err error)
	Redo(*state.Context) (err error)
}

func NewHistory(sb smartblock.SmartBlock) IHistory {
	return &history{sb}
}

type history struct {
	smartblock.SmartBlock
}

func (h *history) Undo(ctx *state.Context) (err error) {
	action, err := h.History().Previous()
	if err != nil {
		return
	}

	s := h.NewStateCtx(ctx)

	for _, b := range action.Add {
		s.Unlink(b.Model().Id)
	}
	for _, b := range action.Remove {
		s.Set(b.Copy())
	}
	for _, b := range action.Change {
		s.Set(b.Before.Copy())
	}

	return h.Apply(s, smartblock.NoHistory)
}

func (h *history) Redo(ctx *state.Context) (err error) {
	action, err := h.History().Next()
	if err != nil {
		return
	}

	s := h.NewStateCtx(ctx)

	for _, b := range action.Add {
		s.Set(b.Copy())
	}
	for _, b := range action.Remove {
		s.Unlink(b.Model().Id)
	}
	for _, b := range action.Change {
		s.Set(b.After.Copy())
	}
	return h.Apply(s, smartblock.NoHistory)
}
