package stext

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
)

type Text interface {
	UpdateTextBlocks(ids []string, showEvent bool, apply func(t text.Block) error) error
	Split(id string, pos int32, style model.BlockContentTextStyle) (newId string, err error)
	Merge(firstId, secondId string) (err error)
}

func NewText(sb smartblock.SmartBlock) Text {
	return &textImpl{sb}
}

type textImpl struct {
	smartblock.SmartBlock
}

func (t *textImpl) UpdateTextBlocks(ids []string, showEvent bool, apply func(t text.Block) error) error {
	s := t.NewState()
	for _, id := range ids {
		tb, err := getText(s, id)
		if err != nil {
			return err
		}
		if err = apply(tb); err != nil {
			return err
		}
	}
	if showEvent {
		return t.Apply(s)
	}
	return t.Apply(s, smartblock.NoEvent)
}

func (t *textImpl) Split(id string, pos int32, style model.BlockContentTextStyle) (newId string, err error) {
	s := t.NewState()
	tb, err := getText(s, id)
	if err != nil {
		return
	}
	new, err := tb.Split(pos)
	if err != nil {
		return
	}
	new.Model().GetText().Style = style
	s.Add(new)
	newId = new.Model().Id
	if err = s.InsertTo(id, model.Block_Top, newId); err != nil {
		return
	}
	if err = t.Apply(s); err != nil {
		return
	}
	return
}

func (t *textImpl) Merge(firstId, secondId string) (err error) {
	s := t.NewState()
	first, err := getText(s, firstId)
	if err != nil {
		return
	}
	second, err := getText(s, secondId)
	if err != nil {
		return
	}
	if err = first.Merge(second); err != nil {
		return
	}
	s.Remove(second.Model().Id)
	return t.Apply(s)
}

func getText(s *state.State, id string) (text.Block, error) {
	b := s.Get(id)
	if b == nil {
		return nil, smartblock.ErrSimpleBlockNotFound
	}
	if tb, ok := b.(text.Block); ok {
		return tb, nil
	}
	return nil, fmt.Errorf("block '%s' not a text block", id)
}
