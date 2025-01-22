package chatobject

import (
	"slices"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

type subscription struct {
	spaceId     string
	chatId      string
	eventSender event.Sender

	sessionContext session.Context

	eventsBuffer []*pb.EventMessage

	ids []string
}

func newSubscription(spaceId string, chatId string, eventSender event.Sender) *subscription {
	return &subscription{
		spaceId:     spaceId,
		chatId:      chatId,
		eventSender: eventSender,
	}
}

func (s *subscription) subscribe(subId string) {
	if !slices.Contains(s.ids, subId) {
		s.ids = append(s.ids, subId)
	}
}

func (s *subscription) unsubscribe(subId string) {
	s.ids = slice.Remove(s.ids, subId)
}

func (s *subscription) isActive() bool {
	return len(s.ids) > 0
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

	ev := &pb.Event{
		ContextId: s.chatId,
		Messages:  slices.Clone(s.eventsBuffer),
	}
	if s.sessionContext != nil {
		s.sessionContext.SetMessages(s.chatId, slices.Clone(s.eventsBuffer))
		s.eventSender.BroadcastToOtherSessions(s.sessionContext.ID(), ev)
		s.sessionContext = nil
	} else if s.isActive() {
		s.eventSender.Broadcast(ev)
	}
}

func (s *subscription) add(message *model.ChatMessage) {
	if !s.canSend() {
		return
	}
	ev := &pb.EventChatAdd{
		Id:      message.Id,
		Message: message,
		OrderId: message.OrderId,
		SubIds:  slices.Clone(s.ids),
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatAdd{
		ChatAdd: ev,
	}))
}

func (s *subscription) delete(messageId string) {
	ev := &pb.EventChatDelete{
		Id:     messageId,
		SubIds: slices.Clone(s.ids),
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
		SubIds:  slices.Clone(s.ids),
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
		SubIds:    slices.Clone(s.ids),
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatUpdateReactions{
		ChatUpdateReactions: ev,
	}))
}

func (s *subscription) canSend() bool {
	if s.sessionContext != nil {
		return true
	}
	if !s.isActive() {
		return false
	}
	return true
}
