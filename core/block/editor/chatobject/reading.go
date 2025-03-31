package chatobject

import (
	"context"
	"fmt"

	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type CounterType int

const (
	CounterTypeMessage = CounterType(iota)
	CounterTypeMention
)

type readHandler interface {
	getUnreadFilter() query.Filter
	getMessagesFilter() query.Filter
	getDiffManagerName() string
	getReadKey() string
	readModifier(value bool) query.Modifier

	readMessages(newOldestOrderId string, idsModified []string)
	unreadMessages(newOldestOrderId string, lastAddedAt int64, msgIds []string)
}

type readMessagesHandler struct {
	subscription *subscription
}

func (h *readMessagesHandler) getUnreadFilter() query.Filter {
	return query.Not{
		Filter: query.Key{Path: []string{readKey}, Filter: query.NewComp(query.CompOpEq, true)},
	}
}

func (h *readMessagesHandler) getMessagesFilter() query.Filter {
	return nil
}

func (h *readMessagesHandler) getDiffManagerName() string {
	return diffManagerMessages
}

func (h *readMessagesHandler) getReadKey() string {
	return readKey
}

func (h *readMessagesHandler) readMessages(newOldestOrderId string, idsModified []string) {
	h.subscription.updateChatState(func(state *model.ChatState) {
		state.Messages.OldestOrderId = newOldestOrderId
	})
	h.subscription.updateMessageRead(idsModified, true)
}

func (h *readMessagesHandler) unreadMessages(newOldestOrderId string, lastAddedAt int64, msgIds []string) {
	h.subscription.updateChatState(func(state *model.ChatState) {
		state.Messages.OldestOrderId = newOldestOrderId
		state.DbTimestamp = int64(lastAddedAt)
	})
	h.subscription.updateMessageRead(msgIds, false)
}

func (h *readMessagesHandler) readModifier(value bool) query.Modifier {
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
	subscription *subscription
}

func (h *readMentionsHandler) getUnreadFilter() query.Filter {
	return query.And{
		query.Key{Path: []string{hasMentionKey}, Filter: query.NewComp(query.CompOpEq, true)},
		query.Key{Path: []string{mentionReadKey}, Filter: query.NewComp(query.CompOpEq, false)},
	}
}

func (h *readMentionsHandler) getMessagesFilter() query.Filter {
	return query.Key{Path: []string{hasMentionKey}, Filter: query.NewComp(query.CompOpEq, true)}
}

func (h *readMentionsHandler) getDiffManagerName() string {
	return diffManagerMentions
}

func (h *readMentionsHandler) getReadKey() string {
	return mentionReadKey
}

func (h *readMentionsHandler) readMessages(newOldestOrderId string, idsModified []string) {
	h.subscription.updateChatState(func(state *model.ChatState) {
		state.Mentions.OldestOrderId = newOldestOrderId
	})
	h.subscription.updateMentionRead(idsModified, true)
}

func (h *readMentionsHandler) unreadMessages(newOldestOrderId string, lastAddedAt int64, msgIds []string) {
	h.subscription.updateChatState(func(state *model.ChatState) {
		state.Mentions.OldestOrderId = newOldestOrderId
		state.DbTimestamp = int64(lastAddedAt)
	})
	h.subscription.updateMentionRead(msgIds, false)
}

func (h *readMentionsHandler) readModifier(value bool) query.Modifier {
	return query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		if v.GetBool(hasMentionKey) {
			oldValue := v.GetBool(h.getReadKey())
			if oldValue != value {
				v.Set(h.getReadKey(), arenaNewBool(a, value))
				return v, true, nil
			}
		}
		return v, false, nil
	})
}

func newReadHandler(counterType CounterType, subscription *subscription) readHandler {
	switch counterType {
	case CounterTypeMessage:
		return &readMessagesHandler{subscription: subscription}
	case CounterTypeMention:
		return &readMentionsHandler{subscription: subscription}
	default:
		panic("unknown counter type")
	}
}

func (s *storeObject) MarkReadMessages(ctx context.Context, afterOrderId, beforeOrderId string, lastAddedMessageTimestamp int64, counterType CounterType) error {
	handler := newReadHandler(counterType, s.subscription)
	// 1. select all messages with orderId < beforeOrderId and addedTime < lastDbState
	// 2. use the last(by orderId) message id as lastHead
	// 3. update the MarkSeenHeads
	// 2. mark messages as read in the DB

	msgs, err := s.repository.getUnreadMessageIdsInRange(ctx, afterOrderId, beforeOrderId, lastAddedMessageTimestamp, handler)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	// mark the whole tree as seen from the current message
	return s.storeSource.MarkSeenHeads(ctx, handler.getDiffManagerName(), msgs)
}

func (s *storeObject) MarkMessagesAsUnread(ctx context.Context, afterOrderId string, counterType CounterType) error {
	txn, err := s.repository.writeTx(ctx)
	if err != nil {
		return fmt.Errorf("create tx: %w", err)
	}
	defer txn.Rollback()

	handler := newReadHandler(counterType, s.subscription)
	messageIds, err := s.repository.getReadMessagesAfter(txn.Context(), afterOrderId, handler)
	if err != nil {
		return fmt.Errorf("get read messages: %w", err)
	}

	if len(messageIds) == 0 {
		return nil
	}

	idsModified := s.repository.setReadFlag(txn.Context(), s.Id(), messageIds, handler, false)
	if len(idsModified) == 0 {
		return nil
	}

	newOldestOrderId, err := s.repository.getOldestOrderId(txn.Context(), handler)
	if err != nil {
		return fmt.Errorf("get oldest order id: %w", err)
	}

	lastAdded, err := s.repository.getLastAddedDate(txn.Context())
	if err != nil {
		return fmt.Errorf("get last added date: %w", err)
	}

	handler.unreadMessages(newOldestOrderId, lastAdded, idsModified)
	s.subscription.flush()

	seenHeads, err := s.seenHeadsCollector.collectSeenHeads(ctx, afterOrderId)
	if err != nil {
		return fmt.Errorf("get seen heads: %w", err)
	}
	err = s.storeSource.InitDiffManager(ctx, diffManagerMessages, seenHeads)
	if err != nil {
		return fmt.Errorf("init diff manager: %w", err)
	}
	err = s.storeSource.StoreSeenHeads(txn.Context(), diffManagerMessages)
	if err != nil {
		return fmt.Errorf("store seen heads: %w", err)
	}

	return txn.Commit()
}

func (s *storeObject) markReadMessages(changeIds []string, handler readHandler) {
	if len(changeIds) == 0 {
		return
	}

	txn, err := s.repository.writeTx(s.componentCtx)
	if err != nil {
		log.With(zap.Error(err)).Error("markReadMessages: start write tx")
		return
	}
	defer txn.Commit()

	idsModified := s.repository.setReadFlag(txn.Context(), s.Id(), changeIds, handler, true)

	if len(idsModified) > 0 {
		newOldestOrderId, err := s.repository.getOldestOrderId(txn.Context(), handler)
		if err != nil {
			log.Error("markReadMessages: get oldest order id", zap.Error(err))
			err = txn.Rollback()
			if err != nil {
				log.Error("markReadMessages: rollback transaction", zap.Error(err))
			}
		}

		handler.readMessages(newOldestOrderId, idsModified)
		s.subscription.flush()
	}
}
