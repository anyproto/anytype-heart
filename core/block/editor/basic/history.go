package basic

import "github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"

type IHistory interface {
	Undo() (err error)
	Redo() (err error)
}

func NewHistory(sb smartblock.SmartBlock) IHistory {
	return &history{sb}
}

type history struct {
	smartblock.SmartBlock
}

func (h *history) Undo() (err error) {
	action, err := h.History().Previous()
	if err != nil {
		return
	}

	s := h.NewState()

	for _, b := range action.Add {
		s.Remove(b.Model().Id)
	}
	for _, b := range action.Remove {
		s.Set(b)
	}
	for _, b := range action.Change {
		s.Set(b.Before)
	}

	return h.Apply(s, smartblock.NoHistory)
}

func (h *history) Redo() (err error) {
	action, err := h.History().Next()
	if err != nil {
		return
	}

	s := h.NewState()

	for _, b := range action.Add {
		s.Set(b)
	}
	for _, b := range action.Remove {
		s.Remove(b.Model().Id)
	}
	for _, b := range action.Change {
		s.Set(b.After)
	}
	return h.Apply(s, smartblock.NoHistory)
}
