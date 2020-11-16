package stext

import (
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/link"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
)

var setTextApplyInterval = time.Second * 3

type Text interface {
	UpdateTextBlocks(ctx *state.Context, ids []string, showEvent bool, apply func(t text.Block) error) error
	Split(ctx *state.Context, req pb.RpcBlockSplitRequest) (newId string, err error)
	Merge(ctx *state.Context, firstId, secondId string) (err error)
	SetMark(ctx *state.Context, mark *model.BlockContentTextMark, blockIds ...string) error
	SetText(req pb.RpcBlockSetTextTextRequest) (err error)
	TurnInto(ctx *state.Context, style model.BlockContentTextStyle, ids ...string) error
}

func NewText(sb smartblock.SmartBlock) Text {
	t := &textImpl{SmartBlock: sb, setTextFlushed: make(chan struct{})}
	t.AddHook(t.flushSetTextState, smartblock.HookOnNewState, smartblock.HookOnClose)
	return t
}

var log = logging.Logger("anytype-mw-smartblock")

type textImpl struct {
	smartblock.SmartBlock
	lastSetTextId    string
	lastSetTextState *state.State
	setTextFlushed   chan struct{}
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
	if tb.Model().GetText().Style == model.BlockContentText_Title {
		req.Mode = pb.RpcBlockSplitRequest_TITLE
		req.Style = model.BlockContentText_Paragraph
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
	case pb.RpcBlockSplitRequest_TITLE:
		targetId = template.HeaderLayoutId
		targetPos = model.Block_Bottom
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

func (t *textImpl) newSetTextState(blockId string, ctx *state.Context) *state.State {
	if t.lastSetTextState != nil && t.lastSetTextId == blockId {
		return t.lastSetTextState
	}
	t.lastSetTextId = blockId
	t.lastSetTextState = t.NewStateCtx(ctx)
	go func() {
		select {
		case <-time.After(setTextApplyInterval):
		case <-t.setTextFlushed:
			return
		}
		t.Lock()
		defer t.Unlock()
		t.flushSetTextState()
	}()
	return t.lastSetTextState
}

func (t *textImpl) flushSetTextState() {
	if t.lastSetTextState != nil {
		if err := t.Apply(t.lastSetTextState, smartblock.NoEvent, smartblock.NoHooks); err != nil {
			log.Errorf("can't apply setText state: %v", err)
		}
		t.lastSetTextState = nil
		select {
		case t.setTextFlushed <- struct{}{}:
		default:
		}
	}
}

func (t *textImpl) SetText(req pb.RpcBlockSetTextTextRequest) (err error) {
	ctx := state.NewContext(nil)
	s := t.newSetTextState(req.BlockId, ctx)
	tb, err := getText(s, req.BlockId)
	if err != nil {
		return
	}
	if err = tb.SetText(req.Text, req.Marks); err != nil {
		return
	}
	if _, ok := tb.(text.DetailsBlock); ok {
		if err = t.Apply(s); err != nil {
			return
		}
		msgs := ctx.GetMessages()
		var filtered = msgs[:0]
		for _, msg := range msgs {
			if msg.GetBlockSetText() == nil {
				filtered = append(filtered, msg)
			}
		}
		t.SendEvent(filtered)
		t.lastSetTextState = nil
		t.setTextFlushed <- struct{}{}
		return
	}
	return
}

func (t *textImpl) TurnInto(ctx *state.Context, style model.BlockContentTextStyle, ids ...string) (err error) {
	s := t.NewStateCtx(ctx)

	turnInto := func(b text.Block) {
		b.SetStyle(style)
		// move children up
		switch style {
		case model.BlockContentText_Header1,
			model.BlockContentText_Header2,
			model.BlockContentText_Header3,
			model.BlockContentText_Quote,
			model.BlockContentText_Code:
			if len(b.Model().ChildrenIds) > 0 {
				ids := b.Model().ChildrenIds
				b.Model().ChildrenIds = nil
				if err = s.InsertTo(b.Model().Id, model.Block_Bottom, ids...); err != nil {
					return
				}
			}
		}
		// reset align and color
		switch style {
		case model.BlockContentText_Quote:
			if b.Model().Align == model.Block_AlignCenter {
				b.Model().Align = model.Block_AlignLeft
			}
		case model.BlockContentText_Checkbox,
			model.BlockContentText_Marked,
			model.BlockContentText_Numbered,
			model.BlockContentText_Toggle:
			b.Model().Align = model.Block_AlignLeft
		case model.BlockContentText_Code:
			b.Model().Align = model.Block_AlignLeft
			b.Model().BackgroundColor = ""
			b.Model().GetText().Color = ""
			b.Model().GetText().Marks = &model.BlockContentTextMarks{
				Marks: nil,
			}
		}
	}

	onlyParents := func(ids []string) (parents []string) {
		var childrenIds []string
		for _, id := range ids {
			if b := s.Pick(id); b != nil {
				childrenIds = append(childrenIds, b.Model().ChildrenIds...)
			}
		}
		parents = ids[:0]
		for _, id := range ids {
			if slice.FindPos(childrenIds, id) == -1 {
				parents = append(parents, id)
			}
		}
		return
	}

	switch style {
	case model.BlockContentText_Toggle,
		model.BlockContentText_Checkbox,
		model.BlockContentText_Marked,
		model.BlockContentText_Numbered,
		model.BlockContentText_Header1,
		model.BlockContentText_Header2,
		model.BlockContentText_Header3,
		model.BlockContentText_Code,
		model.BlockContentText_Quote:
		ids = onlyParents(ids)
	}

	for _, id := range ids {
		var textBlock text.Block
		var ok bool
		b := s.Get(id)
		textBlock, ok = b.(text.Block)
		if !ok {
			if linkBlock, ok := b.(link.Block); ok {
				var targetDetails *types.Struct
				if targetId := linkBlock.Model().GetLink().TargetBlockId; targetId != "" {
					result := t.MetaService().FetchDetails([]string{targetId})
					if len(result) > 0 {
						targetDetails = result[0].Details
					}
				}
				textBlock = linkBlock.ToText(targetDetails).(text.Block)
				s.Add(textBlock)
				if err = s.InsertTo(id, model.Block_Replace, textBlock.Model().Id); err != nil {
					return
				}
			}
		}
		if textBlock != nil {
			turnInto(textBlock)
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
