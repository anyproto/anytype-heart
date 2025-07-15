package chatsubscription

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type messagesState struct {
	messages map[string]*model.ChatMessage

	addEvents map[string]*pb.EventChatAdd
}

func newMessagesState() *messagesState {
	return &messagesState{
		messages:  map[string]*model.ChatMessage{},
		addEvents: map[string]*pb.EventChatAdd{},
	}
}

func (s *messagesState) getLastAddedMessage() (*model.ChatMessage, bool) {
	var lastMessage *model.ChatMessage
	for _, m := range s.messages {
		if lastMessage == nil || lastMessage.OrderId < m.OrderId {
			lastMessage = m
		}
	}

	if lastMessage == nil {
		return nil, false
	}

	_, ok := s.addEvents[lastMessage.Id]
	return lastMessage, ok
}

func (s *messagesState) applyEvent(ev *pb.EventMessage) {
	if v := ev.GetChatAdd(); v != nil {
		s.applyAdd(v)
	} else if v := ev.GetChatDelete(); v != nil {
		s.applyDelete(v)
	} else if v := ev.GetChatUpdate(); v != nil {
		s.applyUpdate(v)
	} else if v := ev.GetChatUpdateMentionReadStatus(); v != nil {
		s.applyUpdateMentionReadStatus(v)
	} else if v := ev.GetChatUpdateMessageReadStatus(); v != nil {
		s.applyUpdateMessageReadStatus(v)
	} else if v := ev.GetChatUpdateReactions(); v != nil {
		s.applyUpdateReactions(v)
	} else if v := ev.GetChatUpdateMessageSyncStatus(); v != nil {
		s.applyUpdateMessageSyncStatus(v)
	}
}

func (s *messagesState) applyAdd(ev *pb.EventChatAdd) {
	s.messages[ev.Id] = ev.Message
	s.addEvents[ev.Id] = ev
}

func (s *messagesState) applyDelete(ev *pb.EventChatDelete) {
	delete(s.messages, ev.Id)
}

func (s *messagesState) applyUpdate(ev *pb.EventChatUpdate) {
	s.messages[ev.Id] = ev.Message
}

func (s *messagesState) applyUpdateMentionReadStatus(ev *pb.EventChatUpdateMentionReadStatus) {
	for _, id := range ev.Ids {
		msg, ok := s.messages[id]
		if ok {
			msg.MentionRead = ev.IsRead
		}
	}
}

func (s *messagesState) applyUpdateMessageReadStatus(ev *pb.EventChatUpdateMessageReadStatus) {
	for _, id := range ev.Ids {
		msg, ok := s.messages[id]
		if ok {
			msg.Read = ev.IsRead
		}
	}
}

func (s *messagesState) applyUpdateReactions(ev *pb.EventChatUpdateReactions) {
	msg, ok := s.messages[ev.Id]
	if ok {
		msg.Reactions = ev.Reactions
	}
}

func (s *messagesState) applyUpdateMessageSyncStatus(ev *pb.EventChatUpdateMessageSyncStatus) {
	for _, id := range ev.Ids {
		msg, ok := s.messages[id]
		if ok {
			msg.Synced = ev.IsSynced
		}
	}
}
