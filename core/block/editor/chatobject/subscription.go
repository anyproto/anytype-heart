package chatobject

import (
	"context"
	"slices"
	"sort"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type subscriptionManager struct {
	componentCtx context.Context

	spaceId         string
	chatId          string
	myIdentity      string
	myParticipantId string

	sessionContext session.Context
	eventsBuffer   []*pb.EventMessage

	identityCache *expirable.LRU[string, *domain.Details]
	subscriptions map[string]*subscription

	chatState        *model.ChatState
	needReloadState  bool
	chatStateUpdated bool

	// Deps
	spaceIndex  spaceindex.Store
	eventSender event.Sender
	repository  *repository
}

type subscription struct {
	id               string
	withDependencies bool
}

func (s *storeObject) newSubscriptionManager(fullId domain.FullID, myIdentity string, myParticipantId string) *subscriptionManager {
	return &subscriptionManager{
		componentCtx:    s.componentCtx,
		spaceId:         fullId.SpaceID,
		chatId:          fullId.ObjectID,
		eventSender:     s.eventSender,
		spaceIndex:      s.spaceIndex,
		myIdentity:      myIdentity,
		myParticipantId: myParticipantId,
		identityCache:   expirable.NewLRU[string, *domain.Details](50, nil, time.Minute),
		repository:      s.repository,
		subscriptions:   map[string]*subscription{},
	}
}

// subscribe subscribes to messages. It returns true if there was no subscriptionManager with provided id
func (s *subscriptionManager) subscribe(subId string, withDependencies bool) bool {
	if _, ok := s.subscriptions[subId]; !ok {
		s.subscriptions[subId] = &subscription{
			id:               subId,
			withDependencies: withDependencies,
		}
		s.chatStateUpdated = false
		return true
	}
	return false
}

func (s *subscriptionManager) unsubscribe(subId string) {
	delete(s.subscriptions, subId)
}

func (s *subscriptionManager) isActive() bool {
	return len(s.subscriptions) > 0
}

func (s *subscriptionManager) withDeps() bool {
	for _, sub := range s.subscriptions {
		if sub.withDependencies {
			return true
		}
	}
	return false
}

func (s *subscriptionManager) listSubIds() []string {
	subIds := make([]string, 0, len(s.subscriptions))
	for id := range s.subscriptions {
		subIds = append(subIds, id)
	}
	sort.Strings(subIds)
	return subIds
}

// setSessionContext sets the session context for the current operation
func (s *subscriptionManager) setSessionContext(ctx session.Context) {
	s.sessionContext = ctx
}

func (s *subscriptionManager) loadChatState(ctx context.Context) error {
	state, err := s.repository.loadChatState(ctx)
	if err != nil {
		return err
	}
	s.chatState = state
	return nil
}

func (s *subscriptionManager) getChatState() *model.ChatState {
	return copyChatState(s.chatState)
}

func (s *subscriptionManager) updateChatState(updater func(*model.ChatState) *model.ChatState) {
	s.chatState = updater(s.chatState)
	s.chatStateUpdated = true
}

// flush is called after commiting changes
func (s *subscriptionManager) flush() {
	if !s.canSend() {
		return
	}

	// Reload ChatState after commit
	if s.needReloadState {
		s.updateChatState(func(state *model.ChatState) *model.ChatState {
			newState, err := s.repository.loadChatState(s.componentCtx)
			if err != nil {
				log.Error("failed to reload chat state", zap.Error(err))
				return state
			}
			return newState
		})
		s.needReloadState = false
	}

	events := slices.Clone(s.eventsBuffer)
	s.eventsBuffer = s.eventsBuffer[:0]

	if s.chatStateUpdated {
		events = append(events, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatStateUpdate{ChatStateUpdate: &pb.EventChatUpdateState{
			State:  s.getChatState(),
			SubIds: s.listSubIds(),
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

func (s *subscriptionManager) getIdentityDetails(identity string) (*domain.Details, error) {
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

func (s *subscriptionManager) add(prevOrderId string, message *Message) {
	if !s.canSend() {
		return
	}

	ev := &pb.EventChatAdd{
		Id:           message.Id,
		Message:      message.ChatMessage,
		OrderId:      message.OrderId,
		AfterOrderId: prevOrderId,
		SubIds:       s.listSubIds(),
	}

	if s.withDeps() {
		identityDetails, err := s.getIdentityDetails(message.Creator)
		if err != nil {
			log.Error("get identity details", zap.Error(err))
		} else if identityDetails.Len() > 0 {
			ev.Dependencies = append(ev.Dependencies, identityDetails.ToProto())
		}

		for _, attachment := range message.Attachments {
			attachmentDetails, err := s.spaceIndex.GetDetails(attachment.Target)
			if err != nil {
				log.Error("get attachment details", zap.Error(err))
			} else if attachmentDetails.Len() > 0 {
				ev.Dependencies = append(ev.Dependencies, attachmentDetails.ToProto())
			}
		}
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatAdd{
		ChatAdd: ev,
	}))
}

func (s *subscriptionManager) delete(messageId string) {
	ev := &pb.EventChatDelete{
		Id:     messageId,
		SubIds: s.listSubIds(),
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatDelete{
		ChatDelete: ev,
	}))

	// We can't reload chat state here because Delete operation hasn't been commited yet
	s.needReloadState = true
}

func (s *subscriptionManager) updateFull(message *Message) {
	if !s.canSend() {
		return
	}
	ev := &pb.EventChatUpdate{
		Id:      message.Id,
		Message: message.ChatMessage,
		SubIds:  s.listSubIds(),
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatUpdate{
		ChatUpdate: ev,
	}))
}

func (s *subscriptionManager) updateReactions(message *Message) {
	if !s.canSend() {
		return
	}
	ev := &pb.EventChatUpdateReactions{
		Id:        message.Id,
		Reactions: message.Reactions,
		SubIds:    s.listSubIds(),
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatUpdateReactions{
		ChatUpdateReactions: ev,
	}))
}

// updateMessageRead updates the read status of the messages with the given ids
// read ids should ONLY contain ids if they were actually modified in the DB
func (s *subscriptionManager) updateMessageRead(ids []string, read bool) {
	s.updateChatState(func(state *model.ChatState) *model.ChatState {
		if read {
			state.Messages.Counter -= int32(len(ids))
		} else {
			state.Messages.Counter += int32(len(ids))
		}
		return state
	})

	if !s.canSend() {
		return
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatUpdateMessageReadStatus{
		ChatUpdateMessageReadStatus: &pb.EventChatUpdateMessageReadStatus{
			Ids:    ids,
			IsRead: read,
			SubIds: s.listSubIds(),
		},
	}))
}

func (s *subscriptionManager) updateMentionRead(ids []string, read bool) {
	s.updateChatState(func(state *model.ChatState) *model.ChatState {
		if read {
			state.Mentions.Counter -= int32(len(ids))
		} else {
			state.Mentions.Counter += int32(len(ids))
		}
		return state
	})

	if !s.canSend() {
		return
	}
	s.eventsBuffer = append(s.eventsBuffer, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatUpdateMentionReadStatus{
		ChatUpdateMentionReadStatus: &pb.EventChatUpdateMentionReadStatus{
			Ids:    ids,
			IsRead: read,
			SubIds: s.listSubIds(),
		},
	}))
}

func (s *subscriptionManager) canSend() bool {
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
		LastStateId: state.LastStateId,
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
