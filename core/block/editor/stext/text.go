package stext

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/simple/link"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/slice"
)

var setTextApplyInterval = time.Second * 3

type Text interface {
	UpdateTextBlocks(ctx session.Context, ids []string, showEvent bool, apply func(t text.Block) error) error
	Split(ctx session.Context, req pb.RpcBlockSplitRequest) (newId string, err error)
	Merge(ctx session.Context, firstId, secondId string) (err error)
	SetMark(ctx session.Context, mark *model.BlockContentTextMark, blockIds ...string) error
	SetIcon(ctx session.Context, image, emoji string, blockIds ...string) error
	SetText(ctx session.Context, req pb.RpcBlockTextSetTextRequest) (err error)
	TurnInto(ctx session.Context, style model.BlockContentTextStyle, ids ...string) error
}

func NewText(
	sb smartblock.SmartBlock,
	objectStore spaceindex.Store,
	eventSender event.Sender,
) Text {
	t := &textImpl{
		SmartBlock:     sb,
		objectStore:    objectStore,
		setTextFlushed: make(chan struct{}),
		eventSender:    eventSender,
	}

	t.AddHook(t.flushSetTextState, smartblock.HookOnNewState, smartblock.HookOnClose, smartblock.HookOnBlockClose)
	return t
}

var log = logging.Logger("anytype-mw-smartblock")

type textImpl struct {
	smartblock.SmartBlock
	objectStore spaceindex.Store
	eventSender event.Sender

	lastSetTextId    string
	lastSetTextState *state.State
	setTextFlushed   chan struct{}
}

func (t *textImpl) UpdateTextBlocks(ctx session.Context, ids []string, showEvent bool, apply func(t text.Block) error) error {
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

func (t *textImpl) Split(ctx session.Context, req pb.RpcBlockSplitRequest) (newId string, err error) {
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

	if s.HasParent(req.BlockId, template.HeaderLayoutId) {
		req.Mode = pb.RpcBlockSplitRequest_TITLE
	}
	if req.Mode == pb.RpcBlockSplitRequest_TITLE && s.Pick(template.HeaderLayoutId) == nil {
		req.Mode = pb.RpcBlockSplitRequest_TOP
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
		header := s.Pick(template.HeaderLayoutId).Model()
		pos := slice.FindPos(header.ChildrenIds, req.BlockId)
		if pos != -1 {
			var nextBlock text.Block
			for _, nextBlockId := range header.ChildrenIds[pos+1:] {
				nb, nbErr := getText(s, nextBlockId)
				if nbErr == nil {
					nextBlock = nb
					break
				}
			}
			if nextBlock != nil {
				exText := nextBlock.GetText()
				if strings.TrimSpace(exText) == "" {
					exText = new.(text.Block).GetText()
				} else {
					exText = new.(text.Block).GetText() + "\n" + exText
				}
				nextBlock.SetText(exText, &model.BlockContentTextMarks{})
				targetPos = model.Block_None
				newId = nextBlock.Model().Id
				break
			}
		}
		new.(text.Block).SetStyle(model.BlockContentText_Paragraph)
		targetId = template.HeaderLayoutId
		targetPos = model.Block_Bottom
	}
	if targetPos != model.Block_None {
		if err = s.InsertTo(targetId, targetPos, newId); err != nil {
			return
		}
	}
	if err = t.Apply(s); err != nil {
		return
	}
	return
}

func (t *textImpl) Merge(ctx session.Context, firstId, secondId string) (err error) {
	s := t.NewStateCtx(ctx)

	// Don't merge blocks inside header block
	if s.IsParentOf(template.HeaderLayoutId, secondId) {
		return
	}

	first, err := getText(s, firstId)
	if err != nil {
		return
	}
	second, err := getText(s, secondId)
	if err != nil {
		return
	}

	var mergeOpts []text.MergeOption
	// Don't set style for target block placed inside header block
	if s.IsParentOf(template.HeaderLayoutId, firstId) {
		mergeOpts = append(mergeOpts, text.DontSetStyle())
	}
	if err = first.Merge(second, mergeOpts...); err != nil {
		return
	}
	s.Unlink(second.Model().Id)
	first.Model().ChildrenIds = append(first.Model().ChildrenIds, second.Model().ChildrenIds...)
	if err = t.Apply(s); err != nil {
		return
	}
	return
}

func (t *textImpl) SetMark(ctx session.Context, mark *model.BlockContentTextMark, blockIds ...string) (err error) {
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

// SetIcon sets an icon for the text block with style BlockContentText_Callout(13)
func (t *textImpl) SetIcon(ctx session.Context, image string, emoji string, blockIds ...string) (err error) {
	s := t.NewStateCtx(ctx)
	for _, id := range blockIds {
		tb, err := getText(s, id)
		if err != nil {
			continue
		}

		tb.SetIconImage(image)
		tb.SetIconEmoji(emoji)
	}

	return t.Apply(s)
}

func (t *textImpl) newSetTextState(blockID string, selectedRange *model.Range, ctx session.Context) *state.State {
	if t.lastSetTextState != nil && t.lastSetTextId == blockID {
		return t.lastSetTextState
	}
	if selectedRange != nil {
		t.History().SetCarriageBeforeState(undo.CarriageState{
			BlockID:   blockID,
			RangeFrom: selectedRange.From,
			RangeTo:   selectedRange.To,
		})
	}
	t.lastSetTextId = blockID
	t.lastSetTextState = t.NewStateCtx(ctx)
	go func() {
		select {
		case <-time.After(setTextApplyInterval):
		case <-t.setTextFlushed:
			return
		}
		t.Lock()
		defer t.Unlock()
		t.flushSetTextState(smartblock.ApplyInfo{})
	}()
	return t.lastSetTextState
}

func (t *textImpl) flushSetTextState(_ smartblock.ApplyInfo) error {
	if t.lastSetTextState != nil {
		// We create new context to avoid sending events to the current session
		ctx := session.NewChildContext(t.lastSetTextState.Context())
		t.lastSetTextState.SetContext(ctx)
		t.removeInternalFlags(t.lastSetTextState)
		if err := t.Apply(t.lastSetTextState, smartblock.NoHooks, smartblock.KeepInternalFlags); err != nil {
			log.Errorf("can't apply setText state: %v", err)
		}
		t.sendEvents(ctx)
		t.cancelSetTextState()
	}
	return nil
}

// sendEvents send BlockSetText events only to the other sessions, other events are sent to all sessions
func (t *textImpl) sendEvents(ctx session.Context) {
	msgs := ctx.GetMessages()
	filteredMsgs := msgs[:0]
	for _, msg := range msgs {
		if msg.GetBlockSetText() == nil {
			filteredMsgs = append(filteredMsgs, msg)
		} else {
			t.eventSender.BroadcastToOtherSessions(ctx.ID(), &pb.Event{
				Messages:  []*pb.EventMessage{msg},
				ContextId: t.Id(),
			})
		}
	}
	if len(filteredMsgs) > 0 {
		t.SendEvent(filteredMsgs)
	}
}

func (t *textImpl) cancelSetTextState() {
	if t.lastSetTextState != nil {
		t.lastSetTextState = nil
		select {
		case t.setTextFlushed <- struct{}{}:
		default:
		}
	}
}

func (t *textImpl) SetText(parentCtx session.Context, req pb.RpcBlockTextSetTextRequest) (err error) {
	defer func() {
		if err != nil {
			t.cancelSetTextState()
		}
	}()

	// TODO: GO-2062 Need to refactor text shortening, as it could cut string incorrectly
	// if len(req.Text) > textSizeLimit {
	//	log.With("objectID", t.Id()).Errorf("cannot set text more than %d symbols to single block. Shortening it", textSizeLimit)
	//	req.Text = req.Text[:textSizeLimit]
	// }

	// We create new context to avoid sending events to the current session
	ctx := session.NewChildContext(parentCtx)
	s := t.newSetTextState(req.BlockId, req.SelectedTextRange, ctx)
	wasEmpty := s.IsEmpty(true)

	tb, err := getText(s, req.BlockId)
	if err != nil {
		return
	}
	beforeIds := tb.FillSmartIds(nil)
	tb.SetText(req.Text, req.Marks)
	afterIds := tb.FillSmartIds(nil)
	t.removeInternalFlags(s)

	if _, ok := tb.(text.DetailsBlock); ok || wasEmpty {
		defer t.cancelSetTextState()
		if err = t.Apply(s, smartblock.KeepInternalFlags); err != nil {
			return
		}
		t.sendEvents(ctx)
		return
	}
	if len(beforeIds)+len(afterIds) > 0 {
		sort.Strings(beforeIds)
		sort.Strings(afterIds)
		if !slice.SortedEquals(beforeIds, afterIds) {
			// mentions changed
			t.flushSetTextState(smartblock.ApplyInfo{})
		}
	}

	return
}

func (t *textImpl) TurnInto(ctx session.Context, style model.BlockContentTextStyle, ids ...string) (err error) {
	s := t.NewStateCtx(ctx)

	turnInto := func(b text.Block) {
		b.SetStyle(style)
		// move children up
		switch style {
		case model.BlockContentText_Header1,
			model.BlockContentText_Header2,
			model.BlockContentText_Header3,
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
			model.BlockContentText_Callout,
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

	for _, id := range ids {
		var textBlock text.Block
		var ok bool
		b := s.Get(id)
		textBlock, ok = b.(text.Block)
		if !ok {
			if linkBlock, ok := b.(link.Block); ok {
				var targetDetails *domain.Details
				if targetId := linkBlock.Model().GetLink().TargetBlockId; targetId != "" {
					// nolint:errcheck
					result, _ := t.objectStore.QueryByIds([]string{targetId})
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

func (t *textImpl) isLastTextBlockChanged() (bool, error) {
	if t.lastSetTextState == nil || t.lastSetTextId == "" {
		return true, fmt.Errorf("last state about text block is not saved")
	}
	newTextBlock, err := getText(t.lastSetTextState, t.lastSetTextId)
	if err != nil {
		return true, err
	}
	oldTextBlock := t.lastSetTextState.PickOrigin(t.lastSetTextId)
	messages, err := oldTextBlock.Diff(t.SpaceID(), newTextBlock)
	return len(messages) != 0, err
}

func (t *textImpl) removeInternalFlags(s *state.State) {
	flags := internalflag.NewFromState(s.ParentState())
	if flags.IsEmpty() {
		return
	}
	if textChanged, err := t.isLastTextBlockChanged(); err == nil && !textChanged {
		return
	}
	flags.Remove(model.InternalFlag_editorDeleteEmpty)
	if t.lastSetTextId != state.TitleBlockID && t.lastSetTextId != state.DescriptionBlockID {
		flags.Remove(model.InternalFlag_editorSelectType)
		flags.Remove(model.InternalFlag_editorSelectTemplate)
	}
	flags.AddToState(s)
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
