package chatobject

import (
	"context"
	"errors"
	"fmt"

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

type counterOptions struct {
	unreadFilter    query.Filter
	diffManagerName string
	readKey         string
}

func (o *counterOptions) readModifier(value bool) query.Modifier {
	arena := &anyenc.Arena{}

	valueModifier := arena.NewObject()
	if value {
		valueModifier.Set(o.readKey, arena.NewTrue())
	} else {
		valueModifier.Set(o.readKey, arena.NewFalse())
	}
	obj := arena.NewObject()
	obj.Set("$set", valueModifier)
	return query.MustParseModifier(obj)
}

func newCounterOptions(counterType CounterType) *counterOptions {
	opts := &counterOptions{}

	switch counterType {
	case CounterTypeMessage:
		opts.unreadFilter = unreadMessageFilter()
		opts.diffManagerName = diffManagerMessages
		opts.readKey = readKey
	case CounterTypeMention:
		opts.unreadFilter = unreadMentionFilter()
		opts.diffManagerName = diffManagerMentions
		opts.readKey = mentionReadKey
	default:
		panic("unknown counter type")
	}

	return opts
}

func (s *storeObject) MarkReadMessages(ctx context.Context, afterOrderId, beforeOrderId string, lastAddedMessageTimestamp int64, counterType CounterType) error {
	opts := newCounterOptions(counterType)
	// 1. select all messages with orderId < beforeOrderId and addedTime < lastDbState
	// 2. use the last(by orderId) message id as lastHead
	// 3. update the MarkSeenHeads
	// 2. mark messages as read in the DB

	msgs, err := s.getUnreadMessageIdsInRange(ctx, afterOrderId, beforeOrderId, lastAddedMessageTimestamp, opts)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	// mark the whole tree as seen from the current message
	return s.storeSource.MarkSeenHeads(ctx, opts.diffManagerName, msgs)
}

func (s *storeObject) MarkMessagesAsUnread(ctx context.Context, afterOrderId string, counterType CounterType) error {
	txn, err := s.collection.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("create tx: %w", err)
	}
	defer txn.Rollback()

	msgs, err := s.getReadMessagesAfter(txn, afterOrderId)
	if err != nil {
		return fmt.Errorf("get read messages: %w", err)
	}

	if len(msgs) == 0 {
		return nil
	}

	opts := newCounterOptions(counterType)

	for _, msgId := range msgs {
		_, err := s.collection.UpdateId(txn.Context(), msgId, opts.readModifier(false))
		if err != nil {
			return fmt.Errorf("update message: %w", err)
		}
	}

	newOldestOrderId, err := s.getOldestOrderId(txn, opts)
	if err != nil {
		return fmt.Errorf("get oldest order id: %w", err)
	}

	lastAdded, err := s.getLastAddedDate(txn)
	if err != nil {
		return fmt.Errorf("get last added date: %w", err)
	}

	s.subscription.updateChatState(func(state *model.ChatState) {
		state.Messages.OldestOrderId = newOldestOrderId
		state.DbTimestamp = int64(lastAdded)
	})
	s.subscription.updateReadStatus(msgs, false)
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

func (s *storeObject) getReadMessagesAfter(txn anystore.ReadTx, afterOrderId string) ([]string, error) {
	iter, err := s.collection.Find(query.And{
		query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
		query.Key{Path: []string{readKey}, Filter: query.NewComp(query.CompOpEq, true)},
	}).Iter(txn.Context())
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

func (s *storeObject) getUnreadMessageIdsInRange(ctx context.Context, afterOrderId, beforeOrderId string, lastAddedMessageTimestamp int64, opts *counterOptions) ([]string, error) {
	iter, err := s.collection.Find(
		query.And{
			query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
			query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpLte, beforeOrderId)},
			query.Or{
				query.Not{query.Key{Path: []string{addedKey}, Filter: query.Exists{}}},
				query.Key{Path: []string{addedKey}, Filter: query.NewComp(query.CompOpLte, lastAddedMessageTimestamp)},
			},
			opts.unreadFilter,
		},
	).Iter(ctx)
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

func unreadMessageFilter() query.Filter {
	// Use Not because old messages don't have read key
	return query.Not{
		Filter: query.Key{Path: []string{readKey}, Filter: query.NewComp(query.CompOpEq, true)},
	}
}

func unreadMentionFilter() query.Filter {
	// Use Not because old messages don't have read key
	return query.Not{
		Filter: query.Key{Path: []string{mentionReadKey}, Filter: query.NewComp(query.CompOpEq, true)},
	}
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
	opts := newCounterOptions(counterType)

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

func (s *storeObject) getOldestOrderId(txn anystore.ReadTx, opts *counterOptions) (string, error) {
	unreadQuery := s.collection.Find(opts.unreadFilter).Sort(ascOrder)

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
		return doc.Value().GetObject(orderKey).Get("id").GetString(), nil
	}
	return "", nil
}

func (s *storeObject) countUnreadMessages(txn anystore.ReadTx, opts *counterOptions) (int, error) {
	unreadQuery := s.collection.Find(opts.unreadFilter)

	return unreadQuery.Limit(1).Count(txn.Context())
}

func (s *storeObject) getLastAddedDate(txn anystore.ReadTx) (int, error) {
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
		return doc.Value().GetInt(addedKey), nil
	}
	return 0, nil
}

func (s *storeObject) markReadMessages(changeIds []string, opts *counterOptions) {
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
		res, err := s.collection.UpdateId(txn.Context(), id, opts.readModifier(true))
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
		newOldestOrderId, err := s.getOldestOrderId(txn, opts)
		if err != nil {
			log.Error("markReadMessages: get oldest order id", zap.Error(err))
			err = txn.Rollback()
			if err != nil {
				log.Error("markReadMessages: rollback transaction", zap.Error(err))
			}
		}

		s.subscription.updateChatState(func(state *model.ChatState) {
			state.Messages.OldestOrderId = newOldestOrderId
		})
		s.subscription.updateReadStatus(idsModified, true)
		s.subscription.flush()
	}
}
