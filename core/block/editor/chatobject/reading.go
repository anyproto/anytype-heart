package chatobject

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	anystore "github.com/anyproto/any-store"
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

func readModifier(key string, value bool) query.Modifier {
	arena := &anyenc.Arena{}

	valueModifier := arena.NewObject()
	valueModifier.Set(key, arenaNewBool(arena, value))
	obj := arena.NewObject()
	obj.Set("$set", valueModifier)
	return query.MustParseModifier(obj)
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

	msgs, err := s.getUnreadMessageIdsInRange(ctx, afterOrderId, beforeOrderId, lastAddedMessageTimestamp, handler)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	// mark the whole tree as seen from the current message
	return s.storeSource.MarkSeenHeads(ctx, handler.getDiffManagerName(), msgs)
}

func (s *storeObject) MarkMessagesAsUnread(ctx context.Context, afterOrderId string, counterType CounterType) error {
	txn, err := s.collection.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("create tx: %w", err)
	}
	defer txn.Rollback()

	handler := newReadHandler(counterType, s.subscription)
	msgs, err := s.getReadMessagesAfter(txn, afterOrderId, handler)
	if err != nil {
		return fmt.Errorf("get read messages: %w", err)
	}

	if len(msgs) == 0 {
		return nil
	}

	for _, msgId := range msgs {
		_, err := s.collection.UpdateId(txn.Context(), msgId, readModifier(handler.getReadKey(), false))
		if err != nil {
			return fmt.Errorf("update message: %w", err)
		}
	}

	newOldestOrderId, err := s.getOldestOrderId(txn, handler)
	if err != nil {
		return fmt.Errorf("get oldest order id: %w", err)
	}

	lastAdded, err := s.getLastAddedDate(txn)
	if err != nil {
		return fmt.Errorf("get last added date: %w", err)
	}

	handler.unreadMessages(newOldestOrderId, lastAdded, msgs)
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

func (s *storeObject) getReadMessagesAfter(txn anystore.ReadTx, afterOrderId string, handler readHandler) ([]string, error) {
	filter := query.And{
		query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
		query.Key{Path: []string{handler.getReadKey()}, Filter: query.NewComp(query.CompOpEq, true)},
	}
	if handler.getMessagesFilter() != nil {
		filter = append(filter, handler.getMessagesFilter())
	}

	iter, err := s.collection.Find(filter).Iter(txn.Context())
	if err != nil {
		return nil, fmt.Errorf("init iterator: %w", err)
	}
	defer iter.Close()

	var msgIds []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		msgIds = append(msgIds, doc.Value().GetString("id"))
	}
	return msgIds, iter.Err()
}

func (s *storeObject) getUnreadMessageIdsInRange(ctx context.Context, afterOrderId, beforeOrderId string, lastAddedMessageTimestamp int64, handler readHandler) ([]string, error) {
	qry := query.And{
		query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
		query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpLte, beforeOrderId)},
		query.Or{
			query.Not{query.Key{Path: []string{addedKey}, Filter: query.Exists{}}},
			query.Key{Path: []string{addedKey}, Filter: query.NewComp(query.CompOpLte, strconv.Itoa(int(lastAddedMessageTimestamp)))},
		},
		handler.getUnreadFilter(),
	}
	iter, err := s.collection.Find(qry).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find id: %w", err)
	}
	defer iter.Close()

	var msgIds []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		msgIds = append(msgIds, doc.Value().GetString("id"))
	}
	return msgIds, iter.Err()
}

// initialChatState returns the initial chat state for the chat object from the DB
func (s *storeObject) initialChatState() (*model.ChatState, error) {
	txn, err := s.collection.ReadTx(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	defer txn.Commit()

	messagesState, err := s.initialChatStateByType(txn, CounterTypeMessage)
	if err != nil {
		return nil, fmt.Errorf("get messages state: %w", err)
	}
	mentionsState, err := s.initialChatStateByType(txn, CounterTypeMention)
	if err != nil {
		return nil, fmt.Errorf("get mentions state: %w", err)
	}

	lastAdded, err := s.getLastAddedDate(txn)
	if err != nil {
		return nil, fmt.Errorf("get last added date: %w", err)
	}

	return &model.ChatState{
		Messages:    messagesState,
		Mentions:    mentionsState,
		DbTimestamp: int64(lastAdded),
	}, nil
}

func (s *storeObject) initialChatStateByType(txn anystore.ReadTx, counterType CounterType) (*model.ChatStateUnreadState, error) {
	opts := newReadHandler(counterType, s.subscription)

	oldestOrderId, err := s.getOldestOrderId(txn, opts)
	if err != nil {
		return nil, fmt.Errorf("get oldest order id: %w", err)
	}

	count, err := s.countUnreadMessages(txn, opts)
	if err != nil {
		return nil, fmt.Errorf("update messages: %w", err)
	}

	return &model.ChatStateUnreadState{
		OldestOrderId: oldestOrderId,
		Counter:       int32(count),
	}, nil
}

func (s *storeObject) getOldestOrderId(txn anystore.ReadTx, handler readHandler) (string, error) {
	unreadQuery := s.collection.Find(handler.getUnreadFilter()).Sort(ascOrder)

	iter, err := unreadQuery.Limit(1).Iter(txn.Context())
	if err != nil {
		return "", fmt.Errorf("init iter: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return "", fmt.Errorf("get doc: %w", err)
		}
		orders := doc.Value().GetObject(orderKey)
		if orders != nil {
			return orders.Get("id").GetString(), nil
		}
	}
	return "", nil
}

func (s *storeObject) countUnreadMessages(txn anystore.ReadTx, handler readHandler) (int, error) {
	unreadQuery := s.collection.Find(handler.getUnreadFilter())

	return unreadQuery.Limit(1).Count(txn.Context())
}

func (s *storeObject) getLastAddedDate(txn anystore.ReadTx) (int64, error) {
	lastAddedDate := s.collection.Find(nil).Sort(descAdded).Limit(1)
	iter, err := lastAddedDate.Iter(txn.Context())
	if err != nil {
		return 0, fmt.Errorf("find last added date: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return 0, fmt.Errorf("get doc: %w", err)
		}
		msg, err := unmarshalMessage(doc.Value())
		if err != nil {
			return 0, fmt.Errorf("unmarshal message: %w", err)
		}
		return msg.AddedAt, nil
	}
	return 0, nil
}

func (s *storeObject) markReadMessages(changeIds []string, handler readHandler) {
	if len(changeIds) == 0 {
		return
	}

	txn, err := s.collection.WriteTx(s.componentCtx)
	if err != nil {
		log.With(zap.Error(err)).Error("markReadMessages: start write tx")
		return
	}
	defer txn.Commit()

	var idsModified []string
	for _, id := range changeIds {
		if id == s.Id() {
			// skip tree root
			continue
		}
		res, err := s.collection.UpdateId(txn.Context(), id, readModifier(handler.getReadKey(), true))
		// Not all changes are messages, skip them
		if errors.Is(err, anystore.ErrDocNotFound) {
			continue
		}
		if err != nil {
			log.Error("markReadMessages: update message", zap.Error(err), zap.String("changeId", id), zap.String("chatObjectId", s.Id()))
			continue
		}
		if res.Modified > 0 {
			idsModified = append(idsModified, id)
		}
	}

	if len(idsModified) > 0 {
		newOldestOrderId, err := s.getOldestOrderId(txn, handler)
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
