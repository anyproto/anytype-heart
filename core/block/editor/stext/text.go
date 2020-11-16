package stext

import (
	"fmt"
	"sort"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/text"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var setTextApplyInterval = time.Second * 3

type Text interface {
	UpdateTextBlocks(ctx *state.Context, ids []string, showEvent bool, apply func(t text.Block) error) error
	Split(ctx *state.Context, req pb.RpcBlockSplitRequest) (newId string, err error)
	Merge(ctx *state.Context, firstId, secondId string) (err error)
	SetMark(ctx *state.Context, mark *model.BlockContentTextMark, blockIds ...string) error
	SetText(req pb.RpcBlockSetTextTextRequest) (err error)
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
	beforeIds := tb.FillSmartIds(nil)
	if err = tb.SetText(req.Text, req.Marks); err != nil {
		return
	}
	afterIds := tb.FillSmartIds(nil)

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
	if len(beforeIds)+len(afterIds) > 0 {
		sort.Strings(beforeIds)
		sort.Strings(afterIds)
		if !slice.SortedEquals(beforeIds, afterIds) {
			// mentions changed
			t.flushSetTextState()
		}
	}

	return
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
