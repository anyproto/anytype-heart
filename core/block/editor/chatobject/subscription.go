package chatobject

import (
	"slices"

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

	firstOrderId string
	enabled      bool
}

func newSubscription(spaceId string, chatId string, eventSender event.Sender) *subscription {
	return &subscription{
		spaceId:     spaceId,
		chatId:      chatId,
		eventSender: eventSender,
	}
}

func (s *subscription) subscribe(firstOrderId string) {
	s.firstOrderId = firstOrderId
	s.enabled = true
}

func (s *subscription) close() {
	s.enabled = false
}

// setSessionContext sets the session context for the current operation
func (s *subscription) setSessionContext(ctx session.Context) {
	s.sessionContext = ctx
}

func (s *subscription) flush() {
	defer func() {
		s.eventsBuffer = s.eventsBuffer[:0]
	}()

	if len(s.eventsBuffer) == 0 {
		return
	}

	if s.sessionContext != nil {
		s.sessionContext.SetMessages(s.chatId, slices.Clone(s.eventsBuffer))
		s.sessionContext = nil
	} else if s.enabled {
		s.eventSender.Broadcast(&pb.Event{
			ContextId: s.chatId,
			Messages:  slices.Clone(s.eventsBuffer),
		})
	}
}

func (s *subscription) add(message *model.ChatMessage) {
	if !s.canSend(message) {
		return
	}
	ev := &pb.EventChatAdd{
		Id:      message.Id,
		Message: message,
		OrderId: message.OrderId,
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
	if !s.canSend(message) {
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
	if !s.canSend(message) {
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

func (s *subscription) canSend(message *model.ChatMessage) bool {
	if s.sessionContext != nil {
		return true
	}
	if !s.enabled {
		return false
	}
	if s.firstOrderId > message.OrderId {
		return false
	}
	return true
}
