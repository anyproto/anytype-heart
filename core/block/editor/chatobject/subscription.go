package chatobject

import (
	"slices"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type subscription struct {
	chatId      string
	eventSender event.Sender

	eventsBuffer []*pb.EventMessage

	firstOrderId string
	enabled      bool
}

func newSubscription(chatId string, eventSender event.Sender) *subscription {
	return &subscription{
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

func (s *subscription) flush() {
	if !s.enabled {
		return
	}
	if len(s.eventsBuffer) == 0 {
		return
	}
	messages := slices.Clone(s.eventsBuffer)
	s.eventSender.Broadcast(&pb.Event{
		ContextId: s.chatId,
		Messages:  messages,
	})
	s.eventsBuffer = s.eventsBuffer[:0]
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
	s.eventsBuffer = append(s.eventsBuffer, &pb.EventMessage{
		Value: &pb.EventMessageValueOfChatAdd{
			ChatAdd: ev,
		},
	})
}

func (s *subscription) delete(messageId string) {
	ev := &pb.EventChatDelete{
		Id: messageId,
	}
	s.eventsBuffer = append(s.eventsBuffer, &pb.EventMessage{
		Value: &pb.EventMessageValueOfChatDelete{
			ChatDelete: ev,
		},
	})
}

func (s *subscription) updateFull(message *model.ChatMessage) {
	if !s.canSend(message) {
		return
	}
	ev := &pb.EventChatUpdate{
		Id:      message.Id,
		Message: message,
	}
	s.eventsBuffer = append(s.eventsBuffer, &pb.EventMessage{
		Value: &pb.EventMessageValueOfChatUpdate{
			ChatUpdate: ev,
		},
	})
}

func (s *subscription) updateReactions(message *model.ChatMessage) {
	if !s.canSend(message) {
		return
	}
	ev := &pb.EventChatUpdateReactions{
		Id:        message.Id,
		Reactions: message.Reactions,
	}
	s.eventsBuffer = append(s.eventsBuffer, &pb.EventMessage{
		Value: &pb.EventMessageValueOfChatUpdateReactions{
			ChatUpdateReactions: ev,
		},
	})
}

func (s *subscription) canSend(message *model.ChatMessage) bool {
	if !s.enabled {
		return false
	}
	if s.firstOrderId > message.OrderId {
		return false
	}
	return true
}
