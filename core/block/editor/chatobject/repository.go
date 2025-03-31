package chatobject

import (
	"context"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type repository struct {
	collection anystore.Collection
}

func (s *repository) getLastAddedDate(txn anystore.ReadTx) (int64, error) {
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
	txn, err := s.collection.ReadTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	defer txn.Commit()

	messagesState, err := s.loadChatStateByType(txn, CounterTypeMessage)
	if err != nil {
		return nil, fmt.Errorf("get messages state: %w", err)
	}
	mentionsState, err := s.loadChatStateByType(txn, CounterTypeMention)
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
		DbTimestamp: lastAdded,
	}, nil
}

func (s *repository) loadChatStateByType(txn anystore.ReadTx, counterType CounterType) (*model.ChatStateUnreadState, error) {
	opts := newReadHandler(counterType, nil)

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

func (s *repository) getOldestOrderId(txn anystore.ReadTx, handler readHandler) (string, error) {
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

func (s *repository) countUnreadMessages(txn anystore.ReadTx, handler readHandler) (int, error) {
	unreadQuery := s.collection.Find(handler.getUnreadFilter())

	return unreadQuery.Count(txn.Context())
}
