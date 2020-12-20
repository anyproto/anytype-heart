package basic

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

type IHistory interface {
	Undo(*state.Context) (counters pb.RpcBlockUndoCounters, err error)
	Redo(*state.Context) (counters pb.RpcBlockUndoCounters, err error)
}

func NewHistory(sb smartblock.SmartBlock) IHistory {
	return &history{sb}
}

type history struct {
	smartblock.SmartBlock
}

func (h *history) Undo(ctx *state.Context) (counters pb.RpcBlockUndoCounters, err error) {
	s := h.NewStateCtx(ctx)
	action, err := h.History().Previous()
	if err != nil {
		return
	}

	for _, b := range action.Add {
		s.Unlink(b.Model().Id)
	}
	for _, b := range action.Remove {
		s.Set(b.Copy())
	}
	for _, b := range action.Change {
		s.Set(b.Before.Copy())
	}
	if action.Details != nil {
		s.SetDetails(pbtypes.CopyStruct(action.Details.Before))
	}
	if err = h.Apply(s, smartblock.NoHistory); err != nil {
		return
	}
	counters.Undo, counters.Redo = h.History().Counters()
	return
}

func (h *history) Redo(ctx *state.Context) (counters pb.RpcBlockUndoCounters, err error) {
	s := h.NewStateCtx(ctx)
	action, err := h.History().Next()
	if err != nil {
		return
	}

	for _, b := range action.Add {
		s.Set(b.Copy())
	}
	for _, b := range action.Remove {
		s.Unlink(b.Model().Id)
	}
	for _, b := range action.Change {
		s.Set(b.After.Copy())
	}
	if action.Details != nil {
		s.SetDetails(pbtypes.CopyStruct(action.Details.After))
	}
	if err = h.Apply(s, smartblock.NoHistory); err != nil {
		return
	}
	counters.Undo, counters.Redo = h.History().Counters()
	return
}
