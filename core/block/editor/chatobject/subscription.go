package chatobject

import (
	"slices"
	"sync"
	"sync/atomic"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type subscription struct {
	spaceId     string
	chatId      string
	eventSender event.Sender

	sessionContext session.Context

	eventsBuffer []*pb.EventMessage

	enabled   atomic.Bool
	chatState *model.ChatState
	sync.Mutex
}

func newSubscription(spaceId string, chatId string, eventSender event.Sender) *subscription {
	return &subscription{
		spaceId:     spaceId,
		chatId:      chatId,
		eventSender: eventSender,
	}
}

func (s *subscription) enable() (wasEnabled bool) {
	return s.enabled.Swap(true) == false
}

func (s *subscription) close() {
	s.enabled.Store(false)
}

// setSessionContext sets the session context for the current operation
func (s *subscription) setSessionContext(ctx session.Context) {
	s.sessionContext = ctx
}

func (s *subscription) flush() *model.ChatState {
	s.Lock()
	// if len(s.eventsBuffer) == 0 {
	//	s.Unlock()
	//	return
	// }
	events := slices.Clone(s.eventsBuffer)
	s.eventsBuffer = s.eventsBuffer[:0]
	chatState := copyChatState(s.chatState)
	s.Unlock()

	events = append(events, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatStateUpdate{ChatStateUpdate: &pb.EventChatUpdateState{
		State: chatState,
	}}))

	ev := &pb.Event{
		ContextId: s.chatId,
		Messages:  events,
	}

	// ????
	if s.sessionContext != nil {
		s.sessionContext.SetMessages(s.chatId, events)
		s.eventSender.BroadcastToOtherSessions(s.sessionContext.ID(), ev)
		s.sessionContext = nil
	} else if s.enabled.Load() {
		s.eventSender.Broadcast(ev)
	}
	return chatState
}

func (s *subscription) add(message *model.ChatMessage) {
	if !s.canSend() {
		return
	}

	ev := &pb.EventChatAdd{
		Id:      message.Id,
		Message: message,
		OrderId: message.OrderId,
	}
	if !message.Read {
		if message.OrderId < s.chatState.Messages.OldestOrderId {
			s.chatState.Messages.OldestOrderId = message.OrderId
		}
		s.chatState.Messages.Counter++
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatAdd{
		ChatAdd: ev,
	}))
}

func (s *subscription) delete(messageId string) {
	ev := &pb.EventChatDelete{
		Id: messageId,
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatDelete{
		ChatDelete: ev,
	}))
}

func (s *subscription) updateFull(message *model.ChatMessage) {
	if !s.canSend() {
		return
	}
	ev := &pb.EventChatUpdate{
		Id:      message.Id,
		Message: message,
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatUpdate{
		ChatUpdate: ev,
	}))
}

func (s *subscription) updateReactions(message *model.ChatMessage) {
	if !s.canSend() {
		return
	}
	ev := &pb.EventChatUpdateReactions{
		Id:        message.Id,
		Reactions: message.Reactions,
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatUpdateReactions{
		ChatUpdateReactions: ev,
	}))
}

// updateReadStatus updates the read status of the messages with the given ids
// read ids should ONLY contain ids if they were actually modified in the DB
func (s *subscription) updateReadStatus(ids []string, read bool) {
	if !s.canSend() {
		return
	}
	if read {
		s.chatState.Messages.Counter -= int32(len(ids))
	} else {
		s.chatState.Messages.Counter += int32(len(ids))
	}

	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatUpdateReadStatus{
		ChatUpdateReadStatus: &pb.EventChatUpdateReadStatus{
			Ids:    ids,
			IsRead: read,
		},
	}))
}

func (s *subscription) canSend() bool {
	if s.sessionContext != nil {
		return true
	}
	if !s.enabled.Load() {
		return false
	}
	return true
}

func copyChatState(state *model.ChatState) *model.ChatState {
	if state == nil {
		return nil
	}
	return &model.ChatState{
		Messages: copyReadState(state.Messages),
		Mentions: copyReadState(state.Mentions),
		DbState:  state.DbState,
	}
}

func copyReadState(state *model.ChatStateUnreadState) *model.ChatStateUnreadState {
	if state == nil {
		return nil
	}
	return &model.ChatStateUnreadState{
		OldestOrderId: state.OldestOrderId,
		Counter:       state.Counter,
	}
}
