package chatsubscription

import (
	"slices"

	"github.com/huandu/skiplist"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type eventAction int

const (
	eventActionAdd eventAction = iota
	eventActionUpdate
	eventActionMentionRead
	eventActionMessageRead
	eventActionUpdateReactions
	eventActionMessageSynced
)

type stateEntry struct {
	msg         *model.ChatMessage
	prevOrderId string
	events      []eventAction
}

type messagesState struct {
	limit int

	messages      *skiplist.SkipList
	messagesByIds map[string]*stateEntry

	outOfWindowEvents map[string]*stateEntry

	deleteIds []string
}

func (s *messagesState) Compare(lhs, rhs interface{}) int {
	lKey, rKey := lhs.(string), rhs.(string)

	lh, ok := s.messagesByIds[lKey]
	if !ok {
		return -1
	}
	rh, ok := s.messagesByIds[rKey]
	if !ok {
		return 1
	}

	if lh.msg.OrderId == rh.msg.OrderId {
		return 0
	}
	if lh.msg.OrderId < rh.msg.OrderId {
		return -1
	}
	return 1
}

func (s *messagesState) CalcScore(key interface{}) float64 {
	return 0
}

func newMessagesState(msgs []*chatmodel.Message, limit int) *messagesState {
	s := &messagesState{
		messagesByIds:     make(map[string]*stateEntry),
		outOfWindowEvents: make(map[string]*stateEntry),
		limit:             limit,
	}
	s.messages = skiplist.New(s)
	for _, msg := range msgs {
		s.applyAddMessage(msg.Id, msg.ChatMessage, "", false)
	}
	return s
}

func (s *messagesState) applyAddMessage(msgId string, msg *model.ChatMessage, prevOrderId string, addEvent bool) {
	front := s.messages.Front()
	if front == nil {
		s.add(msgId, msg, prevOrderId, addEvent)
	} else {
		if s.limit == 0 || s.messages.Len() < s.limit {
			s.add(msgId, msg, prevOrderId, addEvent)
		} else {
			first := s.messagesByIds[front.Key().(string)]
			if msg.OrderId > first.msg.OrderId {
				s.messages.RemoveFront()

				delete(s.messagesByIds, first.msg.Id)
				s.add(msgId, msg, prevOrderId, addEvent)
			}
		}
	}
}

func (s *messagesState) add(msgId string, msg *model.ChatMessage, prevOrderId string, addEvent bool) {
	var events []eventAction
	if addEvent {
		events = append(events, eventActionAdd)
	}
	entry := &stateEntry{
		msg:         msg,
		events:      events,
		prevOrderId: prevOrderId,
	}
	s.messagesByIds[msgId] = entry
	s.messages.Set(msgId, entry)
}

func (s *messagesState) applyDeleteMessage(id string) {
	s.messages.Remove(id)
	delete(s.messagesByIds, id)

	s.deleteIds = append(s.deleteIds, id)
}

func (s *messagesState) applyUpdate(msgId string, msg *model.ChatMessage) {
	prev, ok := s.messagesByIds[msgId]
	if ok {
		prev.msg = msg
		prev.events = append(prev.events, eventActionUpdate)
	} else {
		s.updateOutOfWindowEvent(msgId, func(entry *stateEntry) {
			entry.msg = msg
			entry.events = append(entry.events, eventActionUpdate)
		})
	}
}

func (s *messagesState) applyUpdateMentionReadStatus(msgIds []string, isRead bool) {
	for _, id := range msgIds {
		prev, ok := s.messagesByIds[id]
		if ok {
			prev.msg.MentionRead = isRead
			prev.events = append(prev.events, eventActionMentionRead)
		}
	}
}

func (s *messagesState) applyUpdateMessageReadStatus(msgIds []string, isRead bool) {
	for _, id := range msgIds {
		prev, ok := s.messagesByIds[id]
		if ok {
			prev.msg.Read = isRead
			prev.events = append(prev.events, eventActionMessageRead)
		}
	}
}

func (s *messagesState) applyUpdateReactions(msgId string, msg *model.ChatMessage) {
	prev, ok := s.messagesByIds[msgId]
	if ok {
		prev.msg.Reactions = msg.Reactions
		prev.events = append(prev.events, eventActionUpdateReactions)
	} else {
		s.updateOutOfWindowEvent(msgId, func(entry *stateEntry) {
			entry.msg.Reactions = msg.Reactions
			entry.events = append(entry.events, eventActionUpdateReactions)
		})
	}
}

func (s *messagesState) updateOutOfWindowEvent(msgId string, modifier func(entry *stateEntry)) {
	prev, ok := s.outOfWindowEvents[msgId]
	if ok {
		modifier(prev)
	} else {
		prev = &stateEntry{
			msg: &model.ChatMessage{Id: msgId},
		}
		modifier(prev)
		s.outOfWindowEvents[msgId] = prev
	}
}

func (s *messagesState) applyUpdateMessageSyncStatus(msgIds []string, isSynced bool) {
	for _, id := range msgIds {
		prev, ok := s.messagesByIds[id]
		if ok {
			prev.msg.Synced = isSynced
			prev.events = append(prev.events, eventActionMessageSynced)
		}
	}
}

func (s *messagesState) appendEventsTo(subId string, buf *eventsBuffer) {
	if !slices.Contains(buf.subIds, subId) {
		buf.subIds = append(buf.subIds, subId)
	}

	processEntry := func(entry *stateEntry) {
		prev, ok := buf.eventsByMsgId[entry.msg.Id]
		if !ok {
			prev = &eventsPerMessage{
				msg:         entry.msg,
				prevOrderId: entry.prevOrderId,
				events:      entry.events,
				subIds:      []string{subId},
			}
			buf.eventsByMsgId[entry.msg.Id] = prev
			buf.events = append(buf.events, prev)
		} else {
			prev.subIds = append(prev.subIds, subId)
			prev.addEvents(entry.events)
		}

		entry.events = nil
	}

	for it := s.messages.Front(); it != nil; it = it.Next() {
		entry := it.Value.(*stateEntry)
		if len(entry.events) == 0 {
			continue
		}
		processEntry(entry)
	}

	for msgId, e := range s.outOfWindowEvents {
		processEntry(e)
		delete(s.outOfWindowEvents, msgId)
	}

	for _, id := range s.deleteIds {
		if !slices.Contains(buf.deleteIds, id) {
			buf.deleteIds = append(buf.deleteIds, id)
		}
	}
	s.deleteIds = nil
}

type eventsPerMessage struct {
	msg         *model.ChatMessage
	prevOrderId string
	// TODO Maybe use bitset
	events []eventAction
	subIds []string
}

func (e *eventsPerMessage) addEvents(other []eventAction) {
	for _, ev := range other {
		if !slices.Contains(e.events, ev) {
			e.events = append(e.events, ev)
		}
	}
}

type eventsBuffer struct {
	spaceId       string
	events        []*eventsPerMessage
	eventsByMsgId map[string]*eventsPerMessage
	deleteIds     []string
	subIds        []string
}

func (b *eventsBuffer) buildEvents() []*pb.EventMessage {
	events := make([]*pb.EventMessage, 0, len(b.events))

	for _, ev := range b.events {
		// Add events have the highest priority
		if slices.Contains(ev.events, eventActionAdd) {
			events = append(events, b.buildEvent(ev.msg, eventActionAdd, ev.prevOrderId, ev.subIds))
		} else {
			for _, action := range ev.events {
				events = append(events, b.buildEvent(ev.msg, action, ev.prevOrderId, ev.subIds))
			}
		}
	}

	for _, id := range b.deleteIds {
		events = append(events, event.NewMessage(b.spaceId,
			&pb.EventMessageValueOfChatDelete{
				ChatDelete: &pb.EventChatDelete{
					Id:     id,
					SubIds: b.subIds,
				},
			},
		))
	}

	b.reset()

	return events
}

func (b *eventsBuffer) buildEvent(msg *model.ChatMessage, action eventAction, prevOrderId string, subIds []string) *pb.EventMessage {
	switch action {
	case eventActionAdd:
		return event.NewMessage(b.spaceId,
			&pb.EventMessageValueOfChatAdd{
				ChatAdd: &pb.EventChatAdd{
					Id:           msg.Id,
					OrderId:      msg.OrderId,
					AfterOrderId: prevOrderId,
					Message:      msg,
					SubIds:       subIds,
				},
			})
	case eventActionUpdate:
		return event.NewMessage(b.spaceId,
			&pb.EventMessageValueOfChatUpdate{
				ChatUpdate: &pb.EventChatUpdate{
					Id:      msg.Id,
					Message: msg,
					SubIds:  subIds,
				},
			},
		)
	case eventActionUpdateReactions:
		return event.NewMessage(b.spaceId,
			&pb.EventMessageValueOfChatUpdateReactions{
				ChatUpdateReactions: &pb.EventChatUpdateReactions{
					Id:        msg.Id,
					Reactions: msg.Reactions,
					SubIds:    subIds,
				},
			},
		)
	case eventActionMessageRead:
		return event.NewMessage(b.spaceId,
			&pb.EventMessageValueOfChatUpdateMessageReadStatus{
				ChatUpdateMessageReadStatus: &pb.EventChatUpdateMessageReadStatus{
					Ids:    []string{msg.Id},
					IsRead: msg.Read,
					SubIds: subIds,
				},
			},
		)
	case eventActionMentionRead:
		return event.NewMessage(b.spaceId,
			&pb.EventMessageValueOfChatUpdateMentionReadStatus{
				ChatUpdateMentionReadStatus: &pb.EventChatUpdateMentionReadStatus{
					Ids:    []string{msg.Id},
					IsRead: msg.MentionRead,
					SubIds: subIds,
				},
			},
		)
	case eventActionMessageSynced:
		return event.NewMessage(b.spaceId,
			&pb.EventMessageValueOfChatUpdateMessageSyncStatus{
				ChatUpdateMessageSyncStatus: &pb.EventChatUpdateMessageSyncStatus{
					Ids:      []string{msg.Id},
					IsSynced: msg.Synced,
					SubIds:   subIds,
				},
			},
		)
	default:
		panic("unknown event action")
	}
}

func (b *eventsBuffer) reset() {
	b.events = nil
	for id := range b.eventsByMsgId {
		delete(b.eventsByMsgId, id)
	}
	b.deleteIds = nil
	b.subIds = nil
}
