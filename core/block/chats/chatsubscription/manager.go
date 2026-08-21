package chatsubscription

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// sseSendTimeout bounds how long Flush will wait to deliver an event to a slow
// SSE subscriber before giving up, closing its channel and dropping the
// subscription. The handler observes the closed channel and exits the stream.
const sseSendTimeout = time.Second

type subscriptionManager struct {
	lock sync.Mutex

	componentCtx context.Context

	spaceId         string
	chatId          string
	myIdentity      string
	myParticipantId string

	sessionContext session.Context

	identityCache *expirable.LRU[string, *domain.Details]
	subscriptions map[string]*subscription

	chatStateOrder          int64
	chatState               *model.ChatState
	messageCount            int32
	needReloadState         bool
	needReloadReactionState bool
	chatStateUpdated        bool
	messageCountUpdated     bool

	// Deps
	spaceIndex  spaceindex.Store
	eventSender event.Sender
	repository  chatrepository.Repository
}

type subscription struct {
	id               string
	withDependencies bool

	// couldUseSessionContext determines if client could receive events synchronously in API responses
	couldUseSessionContext bool

	// sseSink, when set, receives events directly instead of going through the event sender
	sseSink chan<- *pb.Event

	state *messagesState
}

func (s *subscriptionManager) Lock() {
	s.lock.Lock()
}

func (s *subscriptionManager) Unlock() {
	s.lock.Unlock()
}

// subscribe subscribes to messagesMap. It returns true if there was no subscriptionManager with provided id
func (s *subscriptionManager) subscribe(req SubscribeLastMessagesRequest, initialMessages []*chatmodel.Message) {
	cloned := make([]*chatmodel.Message, 0, len(initialMessages))
	for _, msg := range initialMessages {
		cloned = append(cloned, msg.Clone())
	}
	st := newMessagesState(cloned, req.Limit)

	s.subscriptions[req.SubId] = &subscription{
		id:                     req.SubId,
		withDependencies:       req.WithDependencies,
		couldUseSessionContext: req.CouldUseSessionContext,
		sseSink:                req.SseSink,
		state:                  st,
	}
	s.chatStateUpdated = false
	s.messageCountUpdated = false
}

func (s *subscriptionManager) unsubscribe(subId string) {
	delete(s.subscriptions, subId)
}

func (s *subscriptionManager) IsActive() bool {
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

// SetSessionContext sets the session context for the current operation
func (s *subscriptionManager) SetSessionContext(ctx session.Context) {
	s.sessionContext = ctx
}

func (s *subscriptionManager) loadChatState(ctx context.Context) error {
	state, err := s.repository.LoadChatState(ctx)
	if err != nil {
		return err
	}
	s.chatState = state
	count, err := s.repository.CountMessages(ctx)
	if err != nil {
		return fmt.Errorf("count messages: %w", err)
	}
	s.messageCount = int32(count)
	return nil
}

func (s *subscriptionManager) GetChatState() *model.ChatState {
	return copyChatState(s.chatState)
}

// ReconcileChatState re-derives the ChatState and message count from the repository — the
// persisted read-flags are the source of truth — and replaces the in-memory copies only when
// they diverged. The in-memory counters can drift from the DB: a change replayed over an
// already-stored message bumps them before the insert is ignored with ErrDocExists, and an
// apply error rolls the DB back after they were already bumped. Managers are also cached
// across object reopens, so drift survives until something reloads. Called under Lock on
// object open, before the first flush, so clients never observe drifted values.
func (s *subscriptionManager) ReconcileChatState() {
	dbState, err := s.repository.LoadChatState(s.componentCtx)
	if err != nil {
		log.Error("reconcile chat state: load from repository", zap.Error(err))
		return
	}
	if !chatStateValuesEqual(s.chatState, dbState) {
		s.UpdateChatState(func(*model.ChatState) *model.ChatState {
			return dbState
		})
	}
	count, err := s.repository.CountMessages(s.componentCtx)
	if err != nil {
		log.Error("reconcile chat state: count messages", zap.Error(err))
		return
	}
	if int32(count) != s.messageCount {
		s.messageCount = int32(count)
		s.messageCountUpdated = true
	}
}

// chatStateValuesEqual compares everything clients consume (counters and watermarks),
// ignoring the local event-ordering field Order.
func chatStateValuesEqual(a, b *model.ChatState) bool {
	return unreadStateEqual(a.GetMessages(), b.GetMessages()) &&
		unreadStateEqual(a.GetMentions(), b.GetMentions()) &&
		a.GetLastStateId() == b.GetLastStateId() &&
		a.GetUnreadReactionOrderId() == b.GetUnreadReactionOrderId()
}

func unreadStateEqual(a, b *model.ChatStateUnreadState) bool {
	return a.GetOldestOrderId() == b.GetOldestOrderId() && a.GetCounter() == b.GetCounter()
}

func (s *subscriptionManager) UpdateChatState(updater func(*model.ChatState) *model.ChatState) {
	s.chatState = updater(s.chatState)
	s.chatStateOrder++
	s.chatState.Order = s.chatStateOrder
	s.chatStateUpdated = true
}

func (s *subscriptionManager) UpdateMessageCount(delta int32) {
	s.messageCount += delta
	if s.messageCount < 0 {
		s.messageCount = 0
	}
	s.messageCountUpdated = true
}

func (s *subscriptionManager) GetMessageCount() int32 {
	return s.messageCount
}

func (s *subscriptionManager) ForceSendingChatState() {
	s.chatStateUpdated = true
	s.messageCountUpdated = true
}

func (s *subscriptionManager) GetLastMessage() (*model.ChatMessage, bool, error) {
	// get the last message from any subscription. It works because we don't have offsets for subscriptions, so
	// it's guaranteed for the last message in a subscription to be the last message in a set of all messages.
	for _, sub := range s.subscriptions {
		last := sub.state.messages.Back()
		if last != nil {
			return proto.Clone(last.Value.(*stateEntry).msg).(*model.ChatMessage), true, nil
		}
	}

	msgs, err := s.repository.GetLastMessages(s.componentCtx, 1)
	if err != nil {
		return nil, false, fmt.Errorf("get last message from repository: %w", err)
	}
	if len(msgs) > 0 {
		return msgs[0].ChatMessage, true, nil
	}
	return nil, false, nil
}

// Flush is called after committing changes. If reloadStateIfNeeded is true and s.needReloadState is true, it reloads state
// and resets s.needReloadState to false
func (s *subscriptionManager) Flush(reloadStateIfNeeded bool) {
	// Reload ChatState after commit
	if s.needReloadState && reloadStateIfNeeded {
		s.UpdateChatState(func(state *model.ChatState) *model.ChatState {
			newState, err := s.repository.LoadChatState(s.componentCtx)
			if err != nil {
				log.Error("failed to reload chat state", zap.Error(err))
				return state
			}
			return newState
		})
		newCount, err := s.repository.CountMessages(s.componentCtx)
		if err != nil {
			log.Error("failed to reload message count", zap.Error(err))
		} else if int32(newCount) != s.messageCount {
			s.messageCount = int32(newCount)
			s.messageCountUpdated = true
		}
		s.needReloadState = false
		s.needReloadReactionState = false
	}

	if s.needReloadReactionState && reloadStateIfNeeded {
		s.UpdateChatState(func(state *model.ChatState) *model.ChatState {
			newOrderId, err := s.repository.GetNewestUnreadReactionOrderId(s.componentCtx)
			if err != nil {
				log.Error("failed to reload reaction state", zap.Error(err))
				return state
			}
			state.UnreadReactionOrderId = newOrderId
			return state
		})
		s.needReloadReactionState = false
	}

	if !s.canSend() {
		return
	}

	buf := &eventsBuffer{
		spaceId:       s.spaceId,
		events:        nil,
		eventsByMsgId: map[string]*eventsPerMessage{},
		deleteIds:     nil,
	}

	for _, sub := range s.subscriptions {
		sub.state.appendEventsTo(sub.id, buf)
	}

	events := buf.buildEvents()

	for _, ev := range events {
		if ev := ev.GetChatAdd(); ev != nil {
			if s.withDeps() {
				s.enrichWithDependencies(ev)
			}
		}
	}

	if s.messageCountUpdated {
		events = append(events, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatUpdateMessageCount{ChatUpdateMessageCount: &pb.EventChatUpdateMessageCount{
			MessageCount: s.messageCount,
			SubIds:       s.listSubIds(),
		}}))
		s.messageCountUpdated = false
	}

	if s.chatStateUpdated {
		events = append(events, event.NewMessage(s.spaceId, &pb.EventMessageValueOfChatStateUpdate{ChatStateUpdate: &pb.EventChatUpdateState{
			State:  s.GetChatState(),
			SubIds: s.listSubIds(),
		}}))
		s.chatStateUpdated = false
	}

	if len(events) == 0 {
		return
	}

	var syncSubIds []string
	var asyncSubIds []string
	type sseSub struct {
		id   string
		sink chan<- *pb.Event
	}
	var sseSubs []sseSub
	for _, sub := range s.subscriptions {
		if sub.sseSink != nil {
			sseSubs = append(sseSubs, sseSub{id: sub.id, sink: sub.sseSink})
		} else if sub.couldUseSessionContext && s.sessionContext != nil {
			syncSubIds = append(syncSubIds, sub.id)
		} else {
			asyncSubIds = append(asyncSubIds, sub.id)
		}
	}

	if len(syncSubIds) > 0 {
		syncEvents := cloneEvents(events)
		eventsSetSubIds(syncSubIds, syncEvents)
		s.sessionContext.SetMessages(s.chatId, append(s.sessionContext.GetMessages(), syncEvents...))

		ev := &pb.Event{
			ContextId: s.chatId,
			Messages:  syncEvents,
		}
		s.eventSender.BroadcastToOtherSessions(s.sessionContext.ID(), ev)
	}

	if len(asyncSubIds) > 0 {
		asyncEvents := cloneEvents(events)
		eventsSetSubIds(asyncSubIds, asyncEvents)
		ev := &pb.Event{
			ContextId: s.chatId,
			Messages:  asyncEvents,
		}
		s.eventSender.Broadcast(ev)
	}

	var droppedSseSubs []string
	for _, ss := range sseSubs {
		sseEvents := cloneEvents(events)
		eventsSetSubIds([]string{ss.id}, sseEvents)
		ev := &pb.Event{
			ContextId: s.chatId,
			Messages:  sseEvents,
		}
		// Try non-blocking first to avoid the timer cost on the common path.
		select {
		case ss.sink <- ev:
			continue
		default:
		}
		// Slow subscriber: wait up to sseSendTimeout, then drop the
		// subscription so we never silently lose events.
		timer := time.NewTimer(sseSendTimeout)
		select {
		case ss.sink <- ev:
			timer.Stop()
		case <-timer.C:
			close(ss.sink)
			droppedSseSubs = append(droppedSseSubs, ss.id)
		}
	}
	for _, id := range droppedSseSubs {
		delete(s.subscriptions, id)
	}
}

func (s *subscriptionManager) enrichWithDependencies(ev *pb.EventChatAdd) {
	deps := s.collectMessageDependencies(ev.Message)
	for _, dep := range deps {
		ev.Dependencies = append(ev.Dependencies, dep.ToProto())
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

func (s *subscriptionManager) Add(prevOrderId string, message *chatmodel.Message) {
	if !s.canSend() {
		return
	}

	for _, sub := range s.subscriptions {
		sub.state.applyAddMessage(message.Id, message.ChatMessage, prevOrderId, true)
	}
}

func (s *subscriptionManager) collectMessageDependencies(message *model.ChatMessage) []*domain.Details {
	var result []*domain.Details

	identityDetails, err := s.getIdentityDetails(message.Creator)
	if err != nil {
		log.Error("get identity details", zap.Error(err))
	} else if identityDetails.Len() > 0 {
		result = append(result, identityDetails)
	}

	for _, attachment := range message.Attachments {
		attachmentDetails, err := s.spaceIndex.GetDetails(attachment.Target)
		if err != nil {
			log.Error("get attachment details", zap.Error(err))
		} else if attachmentDetails.Len() > 0 {
			result = append(result, attachmentDetails)
		}
	}
	return result
}

func (s *subscriptionManager) ForceReloadReactionState() {
	s.needReloadReactionState = true
}

func (s *subscriptionManager) Delete(messageId string) {
	for _, sub := range s.subscriptions {
		sub.state.applyDeleteMessage(messageId)
	}
	// We can't reload chat state here because Delete operation hasn't been commited yet
	s.needReloadState = true
}

func (s *subscriptionManager) UpdateFull(message *chatmodel.Message) {
	if !s.canSend() {
		return
	}

	for _, sub := range s.subscriptions {
		sub.state.applyUpdate(message.Id, message.ChatMessage)
	}
}

func (s *subscriptionManager) UpdateReactions(message *chatmodel.Message) {
	if !s.canSend() {
		return
	}

	for _, sub := range s.subscriptions {
		sub.state.applyUpdateReactions(message.Id, message.ChatMessage)
	}
}

func (s *subscriptionManager) UpdatePinned(message *chatmodel.Message) {
	if !s.canSend() {
		return
	}

	for _, sub := range s.subscriptions {
		sub.state.applyUpdatePinned(message.Id, message.ChatMessage)
	}
}

func (s *subscriptionManager) UpdateSyncStatus(messageIds []string, isSynced bool) {
	if !s.canSend() {
		return
	}

	for _, sub := range s.subscriptions {
		sub.state.applyUpdateMessageSyncStatus(messageIds, isSynced)
	}
}

// updateMessageRead updates the read status of the messagesMap with the given ids
// read ids should ONLY contain ids if they were actually modified in the DB
func (s *subscriptionManager) updateMessageRead(ids []string, read bool) {
	s.UpdateChatState(func(state *model.ChatState) *model.ChatState {
		if read {
			state.Messages.Counter -= int32(len(ids))
		} else {
			state.Messages.Counter += int32(len(ids))
		}
		if state.Messages.Counter < 0 {
			state.Messages.Counter = 0
		}
		return state
	})

	if !s.canSend() {
		return
	}

	if !s.canSend() {
		return
	}

	for _, sub := range s.subscriptions {
		sub.state.applyUpdateMessageReadStatus(ids, read)
	}
}

func (s *subscriptionManager) updateMentionRead(ids []string, read bool) {
	s.UpdateChatState(func(state *model.ChatState) *model.ChatState {
		if read {
			state.Mentions.Counter -= int32(len(ids))
		} else {
			state.Mentions.Counter += int32(len(ids))
		}
		if state.Mentions.Counter < 0 {
			state.Mentions.Counter = 0
		}
		return state
	})

	if !s.canSend() {
		return
	}

	for _, sub := range s.subscriptions {
		sub.state.applyUpdateMentionReadStatus(ids, read)
	}
}

func (s *subscriptionManager) UpdateReactionReadStatus(msgId string, unread bool) {
	if !s.canSend() {
		return
	}
	for _, sub := range s.subscriptions {
		sub.state.applyUpdateReactionReadStatus([]string{msgId}, unread)
	}
}

func (s *subscriptionManager) ReadReactions(newOrderId string, idsModified []string) {
	s.UpdateChatState(func(state *model.ChatState) *model.ChatState {
		state.UnreadReactionOrderId = newOrderId
		return state
	})
	if !s.canSend() {
		return
	}
	for _, sub := range s.subscriptions {
		sub.state.applyUpdateReactionReadStatus(idsModified, false)
	}
}

func (s *subscriptionManager) canSend() bool {
	if s.sessionContext != nil {
		return true
	}
	if !s.IsActive() {
		return false
	}
	return true
}

func (s *subscriptionManager) ReadMessages(newOldestOrderId string, idsModified []string, counterType chatmodel.CounterType) {
	if counterType == chatmodel.CounterTypeMessage {
		s.UpdateChatState(func(state *model.ChatState) *model.ChatState {
			state.Messages.OldestOrderId = newOldestOrderId
			return state
		})
		s.updateMessageRead(idsModified, true)
	} else {
		s.UpdateChatState(func(state *model.ChatState) *model.ChatState {
			state.Mentions.OldestOrderId = newOldestOrderId
			return state
		})
		s.updateMentionRead(idsModified, true)
	}
}

func (s *subscriptionManager) UnreadMessages(newOldestOrderId string, lastStateId string, msgIds []string, counterType chatmodel.CounterType) {
	if counterType == chatmodel.CounterTypeMessage {
		s.UpdateChatState(func(state *model.ChatState) *model.ChatState {
			state.Messages.OldestOrderId = newOldestOrderId
			state.LastStateId = lastStateId
			return state
		})
		s.updateMessageRead(msgIds, false)
	} else {
		s.UpdateChatState(func(state *model.ChatState) *model.ChatState {
			state.Mentions.OldestOrderId = newOldestOrderId
			state.LastStateId = lastStateId
			return state
		})
		s.updateMentionRead(msgIds, false)
	}
}

func copyChatState(state *model.ChatState) *model.ChatState {
	if state == nil {
		return nil
	}
	return &model.ChatState{
		Messages:              copyReadState(state.Messages),
		Mentions:              copyReadState(state.Mentions),
		LastStateId:           state.LastStateId,
		Order:                 state.Order,
		UnreadReactionOrderId: state.UnreadReactionOrderId,
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

func cloneEvents(events []*pb.EventMessage) []*pb.EventMessage {
	res := make([]*pb.EventMessage, 0, len(events))
	for _, ev := range events {
		ev := proto.Clone(ev).(*pb.EventMessage)
		res = append(res, ev)
	}
	return res
}

func eventsSetSubIds(subIds []string, events []*pb.EventMessage) {
	for _, ev := range events {
		if v := ev.GetChatAdd(); v != nil {
			v.SubIds = subIds
		} else if v := ev.GetChatDelete(); v != nil {
			v.SubIds = subIds
		} else if v := ev.GetChatUpdate(); v != nil {
			v.SubIds = subIds
		} else if v := ev.GetChatUpdateMentionReadStatus(); v != nil {
			v.SubIds = subIds
		} else if v := ev.GetChatUpdateMessageReadStatus(); v != nil {
			v.SubIds = subIds
		} else if v := ev.GetChatUpdateReactions(); v != nil {
			v.SubIds = subIds
		} else if v := ev.GetChatStateUpdate(); v != nil {
			v.SubIds = subIds
		} else if v := ev.GetChatUpdateMessageSyncStatus(); v != nil {
			v.SubIds = subIds
		} else if v := ev.GetChatUpdatePinnedStatus(); v != nil {
			v.SubIds = subIds
		} else if v := ev.GetChatUpdateReactionReadStatus(); v != nil {
			v.SubIds = subIds
		} else if v := ev.GetChatUpdateMessageCount(); v != nil {
			v.SubIds = subIds
		}
	}
}
