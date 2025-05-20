package chatrepository

import (
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

type readHandler interface {
	getUnreadFilter() query.Filter
	getMessagesFilter() query.Filter
	getReadKey() string
	readModifier(value bool) query.Modifier
}

type readMessagesHandler struct{}

func (h readMessagesHandler) getUnreadFilter() query.Filter {
	return query.Not{
		Filter: query.Key{Path: []string{chatmodel.ReadKey}, Filter: query.NewComp(query.CompOpEq, true)},
	}
}

func (h readMessagesHandler) getMessagesFilter() query.Filter {
	return nil
}

func (h readMessagesHandler) getReadKey() string {
	return chatmodel.ReadKey
}

func (h readMessagesHandler) readModifier(value bool) query.Modifier {
	return query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		oldValue := v.GetBool(h.getReadKey())
		if oldValue != value {
			v.Set(h.getReadKey(), arenaNewBool(a, value))
			return v, true, nil
		}
		return v, false, nil
	})
}

type readMentionsHandler struct {
}

func (h readMentionsHandler) getUnreadFilter() query.Filter {
	return query.And{
		query.Key{Path: []string{chatmodel.HasMentionKey}, Filter: query.NewComp(query.CompOpEq, true)},
		query.Key{Path: []string{chatmodel.MentionReadKey}, Filter: query.NewComp(query.CompOpEq, false)},
	}
}

func (h readMentionsHandler) getMessagesFilter() query.Filter {
	return query.Key{Path: []string{chatmodel.HasMentionKey}, Filter: query.NewComp(query.CompOpEq, true)}
}

func (h readMentionsHandler) getReadKey() string {
	return chatmodel.MentionReadKey
}

func (h readMentionsHandler) readModifier(value bool) query.Modifier {
	return query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		if v.GetBool(chatmodel.HasMentionKey) {
			oldValue := v.GetBool(h.getReadKey())
			if oldValue != value {
				v.Set(h.getReadKey(), arenaNewBool(a, value))
				return v, true, nil
			}
		}
		return v, false, nil
	})
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
