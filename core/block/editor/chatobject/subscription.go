package chatobject

import (
	"slices"
	"sync"

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

	enabled   bool
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

func (s *subscription) enable() {
	s.enabled = true
}

func (s *subscription) close() {
	s.enabled = false
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

	if s.sessionContext != nil {
		s.sessionContext.SetMessages(s.chatId, events)
		s.eventSender.BroadcastToOtherSessions(s.sessionContext.ID(), ev)
		s.sessionContext = nil
	} else if s.enabled {
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
		s.chatState.Messages.UnreadCounter++
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

func (s *subscription) updateReadStatus(ids []string, read bool) {
	if !s.canSend() {
		return
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
	if !s.enabled {
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
		Replies:  copyReadState(state.Replies),
		DbState:  state.DbState,
	}
}

func copyReadState(state *model.ChatStateUnreadState) *model.ChatStateUnreadState {
	if state == nil {
		return nil
	}
	return &model.ChatStateUnreadState{
		OldestOrderId: state.OldestOrderId,
		UnreadCounter: state.UnreadCounter,
	}
}
