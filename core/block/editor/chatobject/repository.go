package chatobject

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type repository struct {
	collection anystore.Collection
	arenaPool  *anyenc.ArenaPool
}

func (s *repository) writeTx(ctx context.Context) (anystore.WriteTx, error) {
	return s.collection.WriteTx(ctx)
}

func (s *repository) readTx(ctx context.Context) (anystore.ReadTx, error) {
	return s.collection.ReadTx(ctx)
}

func (s *repository) getLastStateId(ctx context.Context) (string, error) {
	lastAddedDate := s.collection.Find(nil).Sort(descStateId).Limit(1)
	iter, err := lastAddedDate.Iter(ctx)
	if err != nil {
		return "", fmt.Errorf("find last added date: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return "", fmt.Errorf("get doc: %w", err)
		}
		msg, err := unmarshalMessage(doc.Value())
		if err != nil {
			return "", fmt.Errorf("unmarshal message: %w", err)
		}
		return msg.StateId, nil
	}
	return "", nil
}

func (s *repository) getPrevOrderId(ctx context.Context, orderId string) (string, error) {
	iter, err := s.collection.Find(query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpLt, orderId)}).
		Sort(descOrder).
		Limit(1).
		Iter(ctx)
	if err != nil {
		return "", fmt.Errorf("init iterator: %w", err)
	}
	defer iter.Close()

	if iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return "", fmt.Errorf("read doc: %w", err)
		}
		prevOrderId := doc.Value().GetString(orderKey, "id")
		return prevOrderId, nil
	}

	return "", nil
}

// initialChatState returns the initial chat state for the chat object from the DB
func (s *repository) loadChatState(ctx context.Context) (*model.ChatState, error) {
	txn, err := s.readTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	defer txn.Commit()

	messagesState, err := s.loadChatStateByType(txn.Context(), CounterTypeMessage)
	if err != nil {
		return nil, fmt.Errorf("get messages state: %w", err)
	}
	mentionsState, err := s.loadChatStateByType(txn.Context(), CounterTypeMention)
	if err != nil {
		return nil, fmt.Errorf("get mentions state: %w", err)
	}

	lastStateId, err := s.getLastStateId(txn.Context())
	if err != nil {
		return nil, fmt.Errorf("get last added date: %w", err)
	}

	return &model.ChatState{
		Messages:    messagesState,
		Mentions:    mentionsState,
		LastStateId: lastStateId,
	}, nil
}

func (s *repository) loadChatStateByType(ctx context.Context, counterType CounterType) (*model.ChatStateUnreadState, error) {
	opts := newReadHandler(counterType, nil)

	oldestOrderId, err := s.getOldestOrderId(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("get oldest order id: %w", err)
	}

	count, err := s.countUnreadMessages(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("update messages: %w", err)
	}

	return &model.ChatStateUnreadState{
		OldestOrderId: oldestOrderId,
		Counter:       int32(count),
	}, nil
}

func (s *repository) getOldestOrderId(ctx context.Context, handler readHandler) (string, error) {
	unreadQuery := s.collection.Find(handler.getUnreadFilter()).Sort(ascOrder)

	iter, err := unreadQuery.Limit(1).Iter(ctx)
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

func (s *repository) countUnreadMessages(ctx context.Context, handler readHandler) (int, error) {
	unreadQuery := s.collection.Find(handler.getUnreadFilter())

	return unreadQuery.Count(ctx)
}

func (s *repository) getReadMessagesAfter(ctx context.Context, afterOrderId string, handler readHandler) ([]string, error) {
	filter := query.And{
		query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
		query.Key{Path: []string{handler.getReadKey()}, Filter: query.NewComp(query.CompOpEq, true)},
	}
	if handler.getMessagesFilter() != nil {
		filter = append(filter, handler.getMessagesFilter())
	}

	iter, err := s.collection.Find(filter).Iter(ctx)
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

func (s *repository) getUnreadMessageIdsInRange(ctx context.Context, afterOrderId, beforeOrderId string, lastStateId string, handler readHandler) ([]string, error) {
	qry := query.And{
		query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
		query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpLte, beforeOrderId)},
		query.Or{
			query.Not{query.Key{Path: []string{stateIdKey}, Filter: query.Exists{}}},
			query.Key{Path: []string{stateIdKey}, Filter: query.NewComp(query.CompOpLte, lastStateId)},
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

func (r *repository) setReadFlag(ctx context.Context, chatObjectId string, msgIds []string, handler readHandler, value bool) []string {
	var idsModified []string
	for _, id := range msgIds {
		if id == chatObjectId {
			// skip tree root
			continue
		}
		res, err := r.collection.UpdateId(ctx, id, handler.readModifier(value))
		// Not all changes are messages, skip them
		if errors.Is(err, anystore.ErrDocNotFound) {
			continue
		}
		if err != nil {
			log.Error("markReadMessages: update message", zap.Error(err), zap.String("changeId", id), zap.String("chatObjectId", chatObjectId))
			continue
		}
		if res.Modified > 0 {
			idsModified = append(idsModified, id)
		}
	}
	return idsModified
}

func (s *repository) getMessages(ctx context.Context, req GetMessagesRequest) ([]*Message, error) {
	var qry anystore.Query
	if req.AfterOrderId != "" {
		operator := query.CompOpGt
		if req.IncludeBoundary {
			operator = query.CompOpGte
		}
		qry = s.collection.Find(query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(operator, req.AfterOrderId)}).Sort(ascOrder).Limit(uint(req.Limit))
	} else if req.BeforeOrderId != "" {
		operator := query.CompOpLt
		if req.IncludeBoundary {
			operator = query.CompOpLte
		}
		qry = s.collection.Find(query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(operator, req.BeforeOrderId)}).Sort(descOrder).Limit(uint(req.Limit))
	} else {
		qry = s.collection.Find(nil).Sort(descOrder).Limit(uint(req.Limit))
	}

	msgs, err := s.queryMessages(ctx, qry)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	return msgs, nil
}

func (s *repository) queryMessages(ctx context.Context, query anystore.Query) ([]*Message, error) {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	iter, err := query.Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find iter: %w", err)
	}
	defer iter.Close()

	var res []*Message
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}

		msg, err := unmarshalMessage(doc.Value())
		if err != nil {
			return nil, fmt.Errorf("unmarshal message: %w", err)
		}
		res = append(res, msg)
	}
	// reverse
	sort.Slice(res, func(i, j int) bool {
		return res[i].OrderId < res[j].OrderId
	})
	return res, nil
}

func (s *repository) hasMyReaction(ctx context.Context, myIdentity string, messageId string, emoji string) (bool, error) {
	doc, err := s.collection.FindId(ctx, messageId)
	if err != nil {
		return false, fmt.Errorf("find message: %w", err)
	}

	msg, err := unmarshalMessage(doc.Value())
	if err != nil {
		return false, fmt.Errorf("unmarshal message: %w", err)
	}
	if v, ok := msg.GetReactions().GetReactions()[emoji]; ok {
		if slices.Contains(v.GetIds(), myIdentity) {
			return true, nil
		}
	}
	return false, nil
}

func (s *repository) getMessagesByIds(ctx context.Context, messageIds []string) ([]*Message, error) {
	txn, err := s.readTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	defer txn.Commit()

	messages := make([]*Message, 0, len(messageIds))
	for _, messageId := range messageIds {
		obj, err := s.collection.FindId(txn.Context(), messageId)
		if errors.Is(err, anystore.ErrDocNotFound) {
			continue
		}
		if err != nil {
			return nil, errors.Join(txn.Commit(), fmt.Errorf("find id: %w", err))
		}
		msg, err := unmarshalMessage(obj.Value())
		if err != nil {
			return nil, errors.Join(txn.Commit(), fmt.Errorf("unmarshal message: %w", err))
		}
		messages = append(messages, msg)
	}
	return messages, txn.Commit()
}

func (s *repository) getLastMessages(ctx context.Context, limit uint) ([]*Message, error) {
	qry := s.collection.Find(nil).Sort(descOrder).Limit(limit)
	return s.queryMessages(ctx, qry)
}
