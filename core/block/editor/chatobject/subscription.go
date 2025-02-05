package chatobject

import (
	"slices"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

const LastMessageSubscriptionId = "lastMessage"

type subscription struct {
	spaceId     string
	chatId      string
	eventSender event.Sender

	sessionContext session.Context

	eventsBuffer []*pb.EventMessage

	spaceIndex spaceindex.Store

	identityCache *expirable.LRU[string, *domain.Details]

	ids []string
}

func newSubscription(spaceId string, chatId string, eventSender event.Sender, spaceIndex spaceindex.Store) *subscription {
	return &subscription{
		spaceId:       spaceId,
		chatId:        chatId,
		eventSender:   eventSender,
		spaceIndex:    spaceIndex,
		identityCache: expirable.NewLRU[string, *domain.Details](50, nil, time.Minute),
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

func (s *subscription) withDeps() bool {
	return slices.Equal(s.ids, []string{LastMessageSubscriptionId})
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

func (s *subscription) getIdentityDetails(identity string) (*domain.Details, error) {
	cached, ok := s.identityCache.Get(identity)
	if ok {
		return cached, nil
	}
	details, err := s.spaceIndex.GetDetails(domain.NewParticipantId(s.spaceId, identity))
	if err != nil {
		return nil, err
	}
	s.identityCache.Add(identity, details)
	return details, nil
}

func (s *subscription) add(prevOrderId string, message *model.ChatMessage) {
	if !s.canSend() {
		return
	}
	ev := &pb.EventChatAdd{
		Id:           message.Id,
		Message:      message,
		OrderId:      message.OrderId,
		AfterOrderId: prevOrderId,
		SubIds:       slices.Clone(s.ids),
	}

	if s.withDeps() {
		identityDetails, err := s.getIdentityDetails(message.Creator)
		if err != nil {
			log.Error("get identity details", zap.Error(err))
		} else {
			ev.Dependencies = append(ev.Dependencies, identityDetails.ToProto())
		}

		for _, attachment := range message.Attachments {
			attachmentDetails, err := s.spaceIndex.GetDetails(attachment.Target)
			if err != nil {
				log.Error("get attachment details", zap.Error(err))
			} else {
				ev.Dependencies = append(ev.Dependencies, attachmentDetails.ToProto())
			}
		}
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
