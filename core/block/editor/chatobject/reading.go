package chatobject

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/anyproto/any-sync/util/slice"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/source"
)

type ReadMessagesRequest struct {
	AfterOrderId  string
	BeforeOrderId string
	LastStateId   string

	CounterType chatmodel.CounterType

	// All forces to read all messages
	All bool
}

func (s *storeObject) MarkReadMessages(ctx context.Context, req ReadMessagesRequest) (markedCount int, err error) {
	// 1. select all messages with orderId < beforeOrderId and addedTime < lastDbState
	// 2. use the last(by orderId) message id as lastHead
	// 3. update the MarkSeenHeads
	// 2. mark messages as read in the DB

	var msgs []string

	if req.All {
		var err error
		msgs, err = s.repository.GetAllUnreadMessages(ctx, req.CounterType)
		if err != nil {
			return 0, fmt.Errorf("get all messages: %w", err)
		}
	} else {
		var err error
		msgs, err = s.repository.GetUnreadMessageIdsInRange(ctx, req.AfterOrderId, req.BeforeOrderId, req.LastStateId, req.CounterType)
		if err != nil {
			return 0, fmt.Errorf("get messages: %w", err)
		}
	}

	// mark the whole tree as seen from the current message
	if err = s.storeSource.MarkSeenHeads(ctx, req.CounterType.DiffManagerName(), msgs); err != nil {
		return
	}
	return len(msgs), nil
}

func (s *storeObject) MarkMessagesAsUnread(ctx context.Context, afterOrderId string, counterType chatmodel.CounterType) error {
	messageIds, err := s.repository.GetReadMessagesAfter(ctx, afterOrderId, counterType)
	if err != nil {
		return fmt.Errorf("get read messages: %w", err)
	}

	if len(messageIds) == 0 {
		return nil
	}

	idsModified, err := s.repository.SetReadFlag(ctx, s.Id(), messageIds, counterType, false)
	if err != nil {
		return fmt.Errorf("set read flag: %w", err)
	}
	if len(idsModified) == 0 {
		return nil
	}

	newOldestOrderId, err := s.repository.GetOldestOrderId(ctx, counterType)
	if err != nil {
		return fmt.Errorf("get oldest order id: %w", err)
	}

	lastAdded, err := s.repository.GetLastStateId(ctx)
	if err != nil {
		return fmt.Errorf("get last added date: %w", err)
	}

	s.subscription.Lock()
	defer s.subscription.Unlock()
	s.subscription.UnreadMessages(newOldestOrderId, lastAdded, idsModified, counterType)
	s.subscription.Flush(false)
	s.applyUnreadCountersLocked()

	seenHeads, err := s.seenHeadsCollector.collectSeenHeads(ctx, afterOrderId)
	if err != nil {
		return fmt.Errorf("get seen heads: %w", err)
	}
	err = s.storeSource.InitDiffManager(ctx, diffManagerMessages, seenHeads)
	if err != nil {
		return fmt.Errorf("init diff manager: %w", err)
	}
	err = s.storeSource.StoreSeenHeads(ctx, diffManagerMessages)
	if err != nil {
		return fmt.Errorf("store seen heads: %w", err)
	}

	return nil
}

func (s *storeObject) markReadMessages(changeIds []string, counterType chatmodel.CounterType) error {
	if len(changeIds) == 0 {
		return nil
	}

	idsModified, err := s.repository.SetReadFlag(s.componentCtx, s.Id(), changeIds, counterType, true)
	if err != nil {
		return fmt.Errorf("set read flag: %w", err)
	}

	if len(idsModified) > 0 {
		newOldestOrderId, err := s.repository.GetOldestOrderId(s.componentCtx, counterType)
		if err != nil {
			return fmt.Errorf("get oldest order id: %w", err)
		}

		s.subscription.Lock()
		defer s.subscription.Unlock()
		s.subscription.ReadMessages(newOldestOrderId, idsModified, counterType)
		s.subscription.Flush(false)
		s.applyUnreadCountersLocked()
	}
	return nil
}

func (s *storeObject) markReadReactions(changeIds []string) error {
	if len(changeIds) == 0 {
		return nil
	}

	var maxOrderId string
	for _, chId := range changeIds {
		ch, err := s.Tree().GetChange(chId)
		if err != nil {
			continue
		}
		if ch.OrderId > maxOrderId {
			maxOrderId = ch.OrderId
		}
	}

	idsModified, err := s.repository.ClearUnreadReactions(s.componentCtx, maxOrderId)
	if err != nil {
		return fmt.Errorf("clear unread reactions: %w", err)
	}
	if len(idsModified) > 0 {
		newOrderId, err := s.repository.GetNewestUnreadReactionOrderId(s.componentCtx)
		if err != nil {
			return fmt.Errorf("get newest unread reaction order id: %w", err)
		}
		s.subscription.Lock()
		defer s.subscription.Unlock()
		s.subscription.ReadReactions(newOrderId, idsModified)
		s.subscription.Flush(false)
		s.applyUnreadCountersLocked()
	}
	return nil
}

func (s *storeObject) MarkReadReactions(ctx context.Context) error {
	changeIds, err := s.repository.GetAllUnreadReactionChangeIds(ctx)
	if err != nil {
		return fmt.Errorf("get unread reaction change ids: %w", err)
	}
	if len(changeIds) == 0 {
		return nil
	}
	return s.storeSource.MarkSeenHeads(ctx, diffManagerReactions, changeIds)
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
		// Find the first record in which the user has got permissions since the last join
		// Example:
		// We have acl: [ 1:noPermissions, 2:reader, 3:noPermission, 4:reader, 5:writer ]
		// Record with id=4 is one that we need
		for i := len(accState.PermissionChanges) - 1; i >= 0; i-- {
			permChange := accState.PermissionChanges[i]

			if permChange.Permission.NoPermissions() {
				break
			} else {
				h.joinedAclRecordId = permChange.RecordId
			}
		}
		break
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
	err = h.source.MarkSeenHeads(ctx, diffManagerReactions, h.headsBeforeJoin)
	if err != nil {
		return fmt.Errorf("mark read reactions: %w", err)
	}
	return nil
}

