package basic

import (
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
)

type IHistory interface {
	Undo(session.Context) (info HistoryInfo, err error)
	Redo(session.Context) (info HistoryInfo, err error)
}

type HistoryInfo struct {
	Counters      pb.RpcObjectUndoRedoCounter
	CarriageState undo.CarriageState
}

func NewHistory(sb smartblock.SmartBlock) IHistory {
	return &history{sb}
}

type history struct {
	smartblock.SmartBlock
}

func (h *history) Undo(ctx session.Context) (info HistoryInfo, err error) {
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
	if action.ObjectTypes != nil {
		ot := make([]domain.TypeKey, len(action.ObjectTypes.Before))
		copy(ot, action.ObjectTypes.Before)
		s.SetObjectTypeKeys(ot)
	}

	if action.Details != nil {
		s.SetDetails(action.Details.Before.Copy())
	}
	s.SetChangeType(domain.HistoryOperation)
	if err = h.Apply(s, smartblock.NoHistory, smartblock.NoRestrictions); err != nil {
		return
	}
	info.Counters.Undo, info.Counters.Redo = h.History().Counters()
	info.CarriageState = action.CarriageInfo.Before
	return
}

func (h *history) Redo(ctx session.Context) (info HistoryInfo, err error) {
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
	if action.ObjectTypes != nil {
		ot := make([]domain.TypeKey, len(action.ObjectTypes.After))
		copy(ot, action.ObjectTypes.After)
		s.SetObjectTypeKeys(ot)
	}
	if action.Details != nil {
		s.SetDetails(action.Details.After.Copy())
	}
	s.SetChangeType(domain.HistoryOperation)
	if err = h.Apply(s, smartblock.NoHistory, smartblock.NoRestrictions); err != nil {
		return
	}
	info.Counters.Undo, info.Counters.Redo = h.History().Counters()
	info.CarriageState = action.CarriageInfo.After
	return
}
