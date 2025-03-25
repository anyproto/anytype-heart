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
	spaceId         string
	chatId          string
	eventSender     event.Sender
	spaceIndex      spaceindex.Store
	myParticipantId string

	sessionContext session.Context

	eventsBuffer  []*pb.EventMessage
	identityCache *expirable.LRU[string, *domain.Details]
	ids           []string

	chatState        *model.ChatState
	chatStateUpdated bool
}

func newSubscription(spaceId string, chatId string, myParticipantId string, eventSender event.Sender, spaceIndex spaceindex.Store) *subscription {
	return &subscription{
		spaceId:         spaceId,
		chatId:          chatId,
		eventSender:     eventSender,
		spaceIndex:      spaceIndex,
		myParticipantId: myParticipantId,
		identityCache:   expirable.NewLRU[string, *domain.Details](50, nil, time.Minute),
	}
}

// subscribe subscribes to messages. It returns true if there was no subscription with provided id
func (s *subscription) subscribe(subId string) bool {
	if !slices.Contains(s.ids, subId) {
		s.ids = append(s.ids, subId)
		s.chatStateUpdated = false
		return true
	}
	return false
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

func (s *subscription) getChatState() *model.ChatState {
	return copyChatState(s.chatState)
}

func (s *subscription) updateChatState(updater func(*model.ChatState)) {
	updater(s.chatState)
	s.chatStateUpdated = true
}

func (s *subscription) flush() {
	if !s.canSend() {
		return
	}

	events := slices.Clone(s.eventsBuffer)
	s.eventsBuffer = s.eventsBuffer[:0]

	if s.chatStateUpdated {
		events = append(events, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatStateUpdate{ChatStateUpdate: &pb.EventChatUpdateState{
			State:  s.getChatState(),
			SubIds: slices.Clone(s.ids),
		}}))
		s.chatStateUpdated = false
	}

	ev := &pb.Event{
		ContextId: s.chatId,
		Messages:  events,
	}

	if s.sessionContext != nil {
		s.sessionContext.SetMessages(s.chatId, events)
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
	s.updateChatState(func(state *model.ChatState) {
		if !message.Read {
			if message.OrderId < state.Messages.OldestOrderId {
				state.Messages.OldestOrderId = message.OrderId
			}
			state.Messages.Counter++

			for _, mark := range message.Message.Marks {
				if mark.Type == model.BlockContentTextMark_Mention && mark.Param == s.myParticipantId {
					state.Mentions.Counter++

					if message.OrderId < state.Mentions.OldestOrderId {
						state.Mentions.OldestOrderId = message.OrderId
					}
					break
				}
			}
		}
		if message.AddedAt > state.DbTimestamp {
			state.DbTimestamp = message.AddedAt
		}
	})

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

// updateReadStatus updates the read status of the messages with the given ids
// read ids should ONLY contain ids if they were actually modified in the DB
func (s *subscription) updateReadStatus(ids []string, read bool) {
	s.updateChatState(func(state *model.ChatState) {
		if read {
			state.Messages.Counter -= int32(len(ids))
		} else {
			state.Messages.Counter += int32(len(ids))
		}
	})

	if !s.canSend() {
		return
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatUpdateReadStatus{
		ChatUpdateReadStatus: &pb.EventChatUpdateReadStatus{
			Ids:    ids,
			IsRead: read,
			SubIds: slices.Clone(s.ids),
		},
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

func copyChatState(state *model.ChatState) *model.ChatState {
	if state == nil {
		return nil
	}
	return &model.ChatState{
		Messages:    copyReadState(state.Messages),
		Mentions:    copyReadState(state.Mentions),
		DbTimestamp: state.DbTimestamp,
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
