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
	for _, message := range messages {
		s.orderToId.Set(message.OrderId, message.Id)
		if s.firstOrderId == "" || s.firstOrderId > message.OrderId {
			s.firstOrderId = message.OrderId
		}
	}
	s.enabled = true
}

func (s *subscription) add(message *model.ChatMessage) {
	if !s.enabled {
		return
	}
	if message.OrderId < s.firstOrderId {
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
	s.eventSender.Broadcast(&pb.Event{
		ContextId: s.chatId,
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfChatAdd{
					ChatAdd: ev,
				},
			},
		},
	})
}

func (s *subscription) update(message *model.ChatMessage) {
	if !s.enabled {
		return
	}
	ev := &pb.EventChatUpdate{
		Id:      message.Id,
		Message: message,
	}
	s.eventSender.Broadcast(&pb.Event{
		ContextId: s.chatId,
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfChatUpdate{
					ChatUpdate: ev,
				},
			},
		},
	})
}
