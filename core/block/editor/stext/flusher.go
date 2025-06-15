package stext

import (
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
)

type flusher struct {
	smartblock.SmartBlock
	eventSender event.Sender

	lastSetTextId    string
	lastSetTextState *state.State
	setTextFlushed   chan struct{}
}

func exampleSetText(req pb.RpcBlockTextSetTextRequest) error {
	// Components on the same object
	t := &textImpl{}
	f := newFlusher()

	// code

	parentCtx := session.NewContext()

	ctx := session.NewChildContext(parentCtx)
	s := f.newSetTextState(req.BlockId, req.SelectedTextRange, ctx)

	detailsBlockChanged, mentionsChanged, err := t.SetText(s, parentCtx, req)

	if err != nil {
		f.cancelSetTextState()
		return err
	}

	if detailsBlockChanged {
		f.cancelSetTextState()
		if err = t.Apply(s, smartblock.KeepInternalFlags); err != nil {
			return err
		}
		f.sendEvents(ctx)
	}

	f.removeInternalFlags(s)
	if mentionsChanged {
		f.flushSetTextState(smartblock.ApplyInfo{})
	}
}

func newFlusher() *flusher {
	return &flusher{
		setTextFlushed: make(chan struct{}),
	}
}

func (t *flusher) cancelSetTextState() {
	if t.lastSetTextState != nil {
		t.lastSetTextState = nil
		select {
		case t.setTextFlushed <- struct{}{}:
		default:
		}
	}
}

func (t *flusher) newSetTextState(blockID string, selectedRange *model.Range, ctx session.Context) *state.State {
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

func (t *flusher) flushSetTextState(_ smartblock.ApplyInfo) error {
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
func (t *flusher) sendEvents(ctx session.Context) {
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

func (t *flusher) isLastTextBlockChanged() (bool, error) {
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

func (t *flusher) removeInternalFlags(s *state.State) {
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
