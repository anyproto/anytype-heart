package chatobject

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
)

func (s *storeObject) MarkReadMessages(ctx context.Context, afterOrderId, beforeOrderId string, lastStateId string, counterType chatmodel.CounterType) error {
	// 1. select all messages with orderId < beforeOrderId and addedTime < lastDbState
	// 2. use the last(by orderId) message id as lastHead
	// 3. update the MarkSeenHeads
	// 2. mark messages as read in the DB

	msgs, err := s.repository.GetUnreadMessageIdsInRange(ctx, afterOrderId, beforeOrderId, lastStateId, counterType)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	// mark the whole tree as seen from the current message
	return s.storeSource.MarkSeenHeads(ctx, counterType.DiffManagerName(), msgs)
}

func (s *storeObject) MarkMessagesAsUnread(ctx context.Context, afterOrderId string, counterType chatmodel.CounterType) error {
	txn, err := s.repository.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("create tx: %w", err)
	}
	var commited bool
	defer func() {
		if !commited {
			_ = txn.Rollback()
		}
	}()
	messageIds, err := s.repository.GetReadMessagesAfter(txn.Context(), afterOrderId, counterType)
	if err != nil {
		return fmt.Errorf("get read messages: %w", err)
	}

	if len(messageIds) == 0 {
		return nil
	}

	idsModified := s.repository.SetReadFlag(txn.Context(), s.Id(), messageIds, counterType, false)
	if len(idsModified) == 0 {
		return nil
	}

	newOldestOrderId, err := s.repository.GetOldestOrderId(txn.Context(), counterType)
	if err != nil {
		return fmt.Errorf("get oldest order id: %w", err)
	}

	lastAdded, err := s.repository.GetLastStateId(txn.Context())
	if err != nil {
		return fmt.Errorf("get last added date: %w", err)
	}

	s.subscription.unreadMessages(newOldestOrderId, lastAdded, idsModified, counterType)
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

	commited = true
	return txn.Commit()
}

func (s *storeObject) markReadMessages(changeIds []string, counterType chatmodel.CounterType) error {
	if len(changeIds) == 0 {
		return nil
	}

	txn, err := s.repository.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	var commited bool
	defer func() {
		if !commited {
			txn.Rollback()
		}
	}()

	idsModified := s.repository.SetReadFlag(txn.Context(), s.Id(), changeIds, counterType, true)

	if len(idsModified) > 0 {
		newOldestOrderId, err := s.repository.GetOldestOrderId(txn.Context(), counterType)
		if err != nil {
			return fmt.Errorf("get oldest order id: %w", err)
		}

		commited = true
		err = txn.Commit()
		if err != nil {
			return fmt.Errorf("commit: %w", err)
		}

		s.subscription.readMessages(newOldestOrderId, idsModified, counterType)
		s.subscription.flush()
	}
	return nil
}
