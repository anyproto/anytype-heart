package stext

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type Text interface {
	UpdateTextBlocks(ctx *state.Context, ids []string, showEvent bool, apply func(t text.Block) error) error
	Split(ctx *state.Context, req pb.RpcBlockSplitRequest) (newId string, err error)
	Merge(ctx *state.Context, firstId, secondId string) (err error)
	SetMark(ctx *state.Context, mark *model.BlockContentTextMark, blockIds ...string) error
}

func NewText(sb smartblock.SmartBlock) Text {
	return &textImpl{sb}
}

type textImpl struct {
	smartblock.SmartBlock
}

func (t *textImpl) UpdateTextBlocks(ctx *state.Context, ids []string, showEvent bool, apply func(t text.Block) error) error {
	s := t.NewStateCtx(ctx)
	for _, id := range ids {
		tb, err := getText(s, id)
		if err != nil {
			continue
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

func (t *textImpl) RangeSplit(ctx *state.Context, id string, rangeFrom int32, rangeTo int32, style model.BlockContentTextStyle) (newId string, err error) {
	s := t.NewStateCtx(ctx)
	tb, err := getText(s, id)
	if err != nil {
		return
	}
	newBlock, err := tb.RangeSplit(rangeFrom, rangeTo, false)
	if err != nil {
		return
	}
	tb.SetStyle(style)
	s.Add(newBlock)
	if err = s.InsertTo(id, model.Block_Top, newBlock.Model().Id); err != nil {
		return
	}
	if err = t.Apply(s); err != nil {
		return
	}
	return newBlock.Model().Id, nil
}

func (t *textImpl) Split(ctx *state.Context, req pb.RpcBlockSplitRequest) (newId string, err error) {
	s := t.NewStateCtx(ctx)
	tb, err := getText(s, req.BlockId)
	if err != nil {
		return
	}
	var from, to int32
	if req.Range != nil {
		from = req.Range.From
		to = req.Range.To
	}
	createTop := req.Mode == pb.RpcBlockSplitRequest_TOP
	new, err := tb.RangeSplit(from, to, createTop)
	if err != nil {
		return
	}
	s.Add(new)
	new.(text.Block).SetStyle(req.Style)
	newId = new.Model().Id
	targetId := req.BlockId
	targetPos := model.Block_Top
	switch req.Mode {
	case pb.RpcBlockSplitRequest_BOTTOM:
		targetPos = model.Block_Bottom
	case pb.RpcBlockSplitRequest_INNER:
		if len(tb.Model().ChildrenIds) == 0 {
			targetPos = model.Block_Inner
		} else {
			targetId = tb.Model().ChildrenIds[0]
			targetPos = model.Block_Top
		}
	}
	if err = s.InsertTo(targetId, targetPos, newId); err != nil {
		return
	}
	if err = t.Apply(s); err != nil {
		return
	}
	return
}

func (t *textImpl) Merge(ctx *state.Context, firstId, secondId string) (err error) {
	s := t.NewStateCtx(ctx)
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
	s.Unlink(second.Model().Id)
	first.Model().ChildrenIds = append(first.Model().ChildrenIds, second.Model().ChildrenIds...)
	return t.Apply(s)
}

func (t *textImpl) SetMark(ctx *state.Context, mark *model.BlockContentTextMark, blockIds ...string) (err error) {
	s := t.NewStateCtx(ctx)
	var reverse = true
	for _, id := range blockIds {
		tb, err := getText(s, id)
		if err != nil {
			continue
		}
		if !tb.HasMarkForAllText(mark) {
			reverse = false
			break
		}
	}
	for _, id := range blockIds {
		tb, err := getText(s, id)
		if err != nil {
			continue
		}
		if reverse {
			tb.RemoveMarkType(mark.Type)
		} else {
			tb.SetMarkForAllText(mark)
		}
	}
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
	return nil, fmt.Errorf("block is not a text block")
}
