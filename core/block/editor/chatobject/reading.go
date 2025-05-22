package chatobject

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/slice"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/source"
	"golang.org/x/exp/slices"
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

	s.subscription.Lock()
	defer s.subscription.Unlock()
	s.subscription.UnreadMessages(newOldestOrderId, lastAdded, idsModified, counterType)
	s.subscription.Flush()

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

		s.subscription.Lock()
		defer s.subscription.Unlock()
		s.subscription.ReadMessages(newOldestOrderId, idsModified, counterType)
		s.subscription.Flush()
	}
	return nil
}

type readStoreTreeHook struct {
	joinedAclRecordId string
	headsBeforeJoin   []string
	currentIdentity   crypto.PubKey
	source            source.Store
}

func (h *readStoreTreeHook) BeforeIteration(ot objecttree.ObjectTree) {
	h.joinedAclRecordId = ot.AclList().Head().Id
	for _, accState := range ot.AclList().AclState().CurrentAccounts() {
		if !accState.PubKey.Equals(h.currentIdentity) {
			continue
		}
		noPermissionsIdx := -1
		for i := len(accState.PermissionChanges) - 1; i >= 0; i-- {
			permChange := accState.PermissionChanges[i]
			if permChange.Permission.NoPermissions() {
				noPermissionsIdx = i
				break
			}
		}

		if noPermissionsIdx == -1 || noPermissionsIdx == len(accState.PermissionChanges)-1 {
			break
		}

		// Get a permission change when user was joined space successfully
		permChange := accState.PermissionChanges[noPermissionsIdx+1]
		h.joinedAclRecordId = permChange.RecordId
	}
}

func (h *readStoreTreeHook) OnIteration(ot objecttree.ObjectTree, change *objecttree.Change) {
	if ok, _ := ot.AclList().IsAfter(h.joinedAclRecordId, change.AclHeadId); ok {
		h.headsBeforeJoin = slice.DiscardFromSlice(h.headsBeforeJoin, func(s string) bool {
			return slices.Contains(change.PreviousIds, s)
		})
		if !slices.Contains(h.headsBeforeJoin, change.Id) {
			h.headsBeforeJoin = append(h.headsBeforeJoin, change.Id)
		}
	}
}

func (h *readStoreTreeHook) AfterDiffManagersInit(ctx context.Context) error {
	err := h.source.MarkSeenHeads(ctx, diffManagerMessages, h.headsBeforeJoin)
	if err != nil {
		return fmt.Errorf("mark read messages: %w", err)
	}
	err = h.source.MarkSeenHeads(ctx, diffManagerMentions, h.headsBeforeJoin)
	if err != nil {
		return fmt.Errorf("mark read mentions: %w", err)
	}
	return nil
}
