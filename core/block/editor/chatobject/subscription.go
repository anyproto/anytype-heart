package chatobject

import (
	"github.com/huandu/skiplist"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type subscription struct {
	chatId      string
	eventSender event.Sender

	orderToId *skiplist.SkipList

	eventsBuffer []*pb.EventMessage

	firstOrderId string
	enabled      bool
}

func newSubscription(chatId string, eventSender event.Sender) *subscription {
	return &subscription{
		chatId:      chatId,
		eventSender: eventSender,

		orderToId: skiplist.New(skiplist.String),
	}
}

func (s *subscription) init(messages []*model.ChatMessage) {
	s.firstOrderId = ""
	s.orderToId = skiplist.New(skiplist.String)
	for _, message := range messages {
		s.orderToId.Set(message.OrderId, message.Id)
		if s.firstOrderId == "" || s.firstOrderId > message.OrderId {
			s.firstOrderId = message.OrderId
		}
	}
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
	s.eventSender.Broadcast(&pb.Event{
		ContextId: s.chatId,
		Messages:  s.eventsBuffer,
	})
	s.eventsBuffer = nil
}

func (s *subscription) add(message *model.ChatMessage) {
	if !s.canSend(message) {
		return
	}

	elem := s.orderToId.Set(message.OrderId, message.Id)
	prev := elem.Prev()
	var afterId string
	if prev != nil {
		afterId = prev.Value.(string)
	}

	ev := &pb.EventChatAdd{
		Id:      message.Id,
		Message: message,
		AfterId: afterId,
	}
	s.eventsBuffer = append(s.eventsBuffer, &pb.EventMessage{
		Value: &pb.EventMessageValueOfChatAdd{
			ChatAdd: ev,
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
