package chatrepository

import (
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

var (
	filterReadTrue  = query.Key{Path: []string{chatmodel.ReadKey}, Filter: query.NewComp(query.CompOpEq, true)}
	filterReadFalse = query.Not{Filter: filterReadTrue}

	filterHasMention        = query.Key{Path: []string{chatmodel.HasMentionKey}, Filter: query.NewComp(query.CompOpEq, true)}
	filterMentionReadTrue   = query.And{filterHasMention, query.Key{Path: []string{chatmodel.MentionReadKey}, Filter: query.NewComp(query.CompOpEq, true)}}
	filterMentionReadFalse  = query.And{filterHasMention, query.Key{Path: []string{chatmodel.MentionReadKey}, Filter: query.NewComp(query.CompOpEq, false)}}
)

type readHandler interface {
	getReadFilter(value bool) query.Filter
	getMessagesFilter() query.Filter
	getReadKey() string
	readModifier(value bool) readModifier
}

type readMessagesHandler struct{}

func (h readMessagesHandler) getReadFilter(value bool) query.Filter {
	if value {
		return filterReadTrue
	}
	// NOT (read == true) to also match documents where the read field is missing
	return filterReadFalse
}

func (h readMessagesHandler) getMessagesFilter() query.Filter {
	return nil
}

func (h readMessagesHandler) getReadKey() string {
	return chatmodel.ReadKey
}

func (h readMessagesHandler) readModifier(value bool) readModifier {
	return &readMessagesModifier{value: value}
}

type readMentionsHandler struct{}

func (h readMentionsHandler) getReadFilter(value bool) query.Filter {
	if value {
		return filterMentionReadTrue
	}
	return filterMentionReadFalse
}

func (h readMentionsHandler) getMessagesFilter() query.Filter {
	return filterHasMention
}

func (h readMentionsHandler) getReadKey() string {
	return chatmodel.MentionReadKey
}

func (h readMentionsHandler) readModifier(value bool) readModifier {
	return &readMentionsModifier{value: value}
}

func newReadHandler(counterType chatmodel.CounterType) readHandler {
	switch counterType {
	case chatmodel.CounterTypeMessage:
		return readMessagesHandler{}
	case chatmodel.CounterTypeMention:
		return readMentionsHandler{}
	default:
		panic("unknown counter type")
	}
}

func arenaNewBool(a *anyenc.Arena, value bool) *anyenc.Value {
	if value {
		return a.NewTrue()
	} else {
		return a.NewFalse()
	}
}

type readModifier interface {
	query.Modifier
	getModifiedIds() []string
}

type readMessagesModifier struct {
	value       bool
	modifiedIds []string
}

func (m *readMessagesModifier) Modify(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
	if v.GetBool(chatmodel.ReadKey) != m.value {
		v.Set(chatmodel.ReadKey, arenaNewBool(a, m.value))
		m.modifiedIds = append(m.modifiedIds, v.GetString("id"))
		return v, true, nil
	}
	return v, false, nil
}

func (m *readMessagesModifier) getModifiedIds() []string {
	return m.modifiedIds
}

type readMentionsModifier struct {
	value       bool
	modifiedIds []string
}

func (m *readMentionsModifier) Modify(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
	if !v.GetBool(chatmodel.HasMentionKey) {
		return v, false, nil
	}
	if v.GetBool(chatmodel.MentionReadKey) != m.value {
		v.Set(chatmodel.MentionReadKey, arenaNewBool(a, m.value))
		m.modifiedIds = append(m.modifiedIds, v.GetString("id"))
		return v, true, nil
	}
	return v, false, nil
}

func (m *readMentionsModifier) getModifiedIds() []string {
	return m.modifiedIds
}
