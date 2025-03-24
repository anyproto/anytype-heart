package chatobject

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/util/slice"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/anystoredebug"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	collectionName      = "chats"
	descOrder           = "-_o.id"
	ascOrder            = "_o.id"
	descAdded           = "-a"
	diffManagerMessages = "messages"
)

var log = logging.Logger("core.block.editor.chatobject").Desugar()

type StoreObject interface {
	smartblock.SmartBlock
	anystoredebug.AnystoreDebug

	AddMessage(ctx context.Context, sessionCtx session.Context, message *model.ChatMessage) (string, error)
	GetMessages(ctx context.Context, req GetMessagesRequest) (*GetMessagesResponse, error)
	GetMessagesByIds(ctx context.Context, messageIds []string) ([]*model.ChatMessage, error)
	EditMessage(ctx context.Context, messageId string, newMessage *model.ChatMessage) error
	ToggleMessageReaction(ctx context.Context, messageId string, emoji string) error
	DeleteMessage(ctx context.Context, messageId string) error
	SubscribeLastMessages(ctx context.Context, subId string, limit int, asyncInit bool) (*SubscribeLastMessagesResponse, error)
	MarkReadMessages(ctx context.Context, afterOrderId string, beforeOrderId string, lastAddedMessageTimestamp int64) error
	MarkMessagesAsUnread(ctx context.Context, afterOrderId string) error
	Unsubscribe(subId string) error
}

type GetMessagesRequest struct {
	AfterOrderId    string
	BeforeOrderId   string
	Limit           int
	IncludeBoundary bool
}

type AccountService interface {
	AccountID() string
}

type seenHeadsCollector interface {
	collectSeenHeads(ctx context.Context, afterOrderId string) ([]string, error)
}

type storeObject struct {
	anystoredebug.AnystoreDebug
	smartblock.SmartBlock
	locker smartblock.Locker

	seenHeadsCollector seenHeadsCollector
	collection         anystore.Collection
	accountService     AccountService
	storeSource        source.Store
	store              *storestate.StoreState
	eventSender        event.Sender
	subscription       *subscription
	crdtDb             anystore.DB
	spaceIndex         spaceindex.Store
	chatHandler        *ChatHandler

	arenaPool          *anyenc.ArenaPool
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

func New(sb smartblock.SmartBlock, accountService AccountService, eventSender event.Sender, crdtDb anystore.DB, spaceIndex spaceindex.Store) StoreObject {
	ctx, cancel := context.WithCancel(context.Background())
	return &storeObject{
		SmartBlock:         sb,
		locker:             sb.(smartblock.Locker),
		accountService:     accountService,
		arenaPool:          &anyenc.ArenaPool{},
		eventSender:        eventSender,
		crdtDb:             crdtDb,
		componentCtx:       ctx,
		componentCtxCancel: cancel,
		spaceIndex:         spaceIndex,
	}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	storeSource, ok := ctx.Source.(source.Store)
	if !ok {
		return fmt.Errorf("source is not a store")
	}
	storeSource.AddDiffManager(diffManagerMessages, s.markReadMessages)

	err := s.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	s.storeSource = storeSource

	s.subscription = newSubscription(s.SpaceID(), s.Id(), s.eventSender, s.spaceIndex)

	s.chatHandler = &ChatHandler{
		subscription:    s.subscription,
		currentIdentity: s.accountService.AccountID(),
	}

	stateStore, err := storestate.New(ctx.Ctx, s.Id(), s.crdtDb, s.chatHandler)
	if err != nil {
		return fmt.Errorf("create state store: %w", err)
	}
	s.store = stateStore
	s.collection, err = s.store.Collection(s.componentCtx, collectionName)
	if err != nil {
		return fmt.Errorf("get s.collection.ction: %w", err)
	}

	s.subscription.chatState, err = s.initialChatState()
	if err != nil {
		return fmt.Errorf("init chat state: %w", err)
	}

	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, s.onUpdate)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}

	s.AnystoreDebug = anystoredebug.New(s.SmartBlock, stateStore)

	s.seenHeadsCollector = newTreeSeenHeadsCollector(s.Tree())

	return nil
}

func (s *storeObject) onUpdate() {
	s.subscription.flush()
}

// initialChatState returns the initial chat state for the chat object from the DB
func (s *storeObject) initialChatState() (*model.ChatState, error) {
	txn, err := s.collection.ReadTx(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	defer txn.Commit()

	oldestOrderId, err := s.getOldestOrderId(txn)
	if err != nil {
		return nil, fmt.Errorf("get oldest order id: %w", err)
	}

	count, err := s.countUnreadMessages(txn)
	if err != nil {
		return nil, fmt.Errorf("update messages: %w", err)
	}

	lastAdded, err := s.getLastAddedDate(txn)
	if err != nil {
		return nil, fmt.Errorf("get last added date: %w", err)
	}

	return &model.ChatState{
		Messages: &model.ChatStateUnreadState{
			OldestOrderId: oldestOrderId,
			Counter:       int32(count),
		},
		// todo: add replies counter
		DbTimestamp: int64(lastAdded),
	}, nil
}

func (s *storeObject) getOldestOrderId(txn anystore.ReadTx) (string, error) {
	unreadQuery := s.collection.Find(unreadFilter()).Sort(ascOrder)

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

func (s *storeObject) countUnreadMessages(txn anystore.ReadTx) (int, error) {
	unreadQuery := s.collection.Find(unreadFilter())

	return unreadQuery.Limit(1).Count(txn.Context())
}

func unreadFilter() query.Filter {
	// Use Not because old messages don't have read key
	return query.Not{
		Filter: query.Key{Path: []string{readKey}, Filter: query.NewComp(query.CompOpEq, true)},
	}
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

func (s *storeObject) markReadMessages(changeIds []string) {
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
		res, err := s.collection.UpdateId(txn.Context(), id, query.MustParseModifier(`{"$set":{"`+readKey+`":true}}`))
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
		newOldestOrderId, err := s.getOldestOrderId(txn)
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

func (s *storeObject) MarkReadMessages(ctx context.Context, afterOrderId, beforeOrderId string, lastAddedMessageTimestamp int64) error {
	// 1. select all messages with orderId < beforeOrderId and addedTime < lastDbState
	// 2. use the last(by orderId) message id as lastHead
	// 3. update the MarkSeenHeads
	// 2. mark messages as read in the DB

	msgs, err := s.getUnreadMessageIdsInRange(ctx, afterOrderId, beforeOrderId, lastAddedMessageTimestamp)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	// mark the whole tree as seen from the current message
	return s.storeSource.MarkSeenHeads(ctx, diffManagerMessages, msgs)
}

func (s *storeObject) MarkMessagesAsUnread(ctx context.Context, afterOrderId string) error {
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

	for _, msgId := range msgs {
		_, err := s.collection.UpdateId(txn.Context(), msgId, query.MustParseModifier(`{"$set":{"`+readKey+`":false}}`))
		if err != nil {
			return fmt.Errorf("update message: %w", err)
		}
	}

	newOldestOrderId, err := s.getOldestOrderId(txn)
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

func (s *storeObject) getUnreadMessageIdsInRange(ctx context.Context, afterOrderId, beforeOrderId string, lastAddedMessageTimestamp int64) ([]string, error) {
	iter, err := s.collection.Find(
		query.And{
			query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
			query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpLte, beforeOrderId)},
			query.Or{
				query.Not{query.Key{Path: []string{addedKey}, Filter: query.Exists{}}},
				query.Key{Path: []string{addedKey}, Filter: query.NewComp(query.CompOpLte, lastAddedMessageTimestamp)},
			},
			unreadFilter(),
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

func (s *storeObject) GetMessagesByIds(ctx context.Context, messageIds []string) ([]*model.ChatMessage, error) {
	txn, err := s.collection.ReadTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	messages := make([]*model.ChatMessage, 0, len(messageIds))
	for _, messageId := range messageIds {
		obj, err := s.collection.FindId(txn.Context(), messageId)
		if errors.Is(err, anystore.ErrDocNotFound) {
			continue
		}
		if err != nil {
			return nil, errors.Join(txn.Commit(), fmt.Errorf("find id: %w", err))
		}
		msg := newMessageWrapper(nil, obj.Value())
		messages = append(messages, msg.toModel())
	}
	return messages, txn.Commit()
}

type GetMessagesResponse struct {
	Messages  []*model.ChatMessage
	ChatState *model.ChatState
}

func (s *storeObject) GetMessages(ctx context.Context, req GetMessagesRequest) (*GetMessagesResponse, error) {
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
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].OrderId < msgs[j].OrderId
	})

	return &GetMessagesResponse{
		Messages:  msgs,
		ChatState: s.subscription.getChatState(),
	}, nil
}

func (s *storeObject) queryMessages(ctx context.Context, query anystore.Query) ([]*model.ChatMessage, error) {
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

	var res []*model.ChatMessage
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}

		message := newMessageWrapper(arena, doc.Value()).toModel()
		res = append(res, message)
	}
	return res, nil
}

func (s *storeObject) AddMessage(ctx context.Context, sessionCtx session.Context, message *model.ChatMessage) (string, error) {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()
	message.Read = true
	obj := marshalModel(arena, message)

	builder := storestate.Builder{}
	err := builder.Create(collectionName, storestate.IdFromChange, obj)
	if err != nil {
		return "", fmt.Errorf("create chat: %w", err)
	}

	s.subscription.setSessionContext(sessionCtx)
	messageId, err := s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("push change: %w", err)
	}
	return messageId, nil
}

func (s *storeObject) DeleteMessage(ctx context.Context, messageId string) error {
	builder := storestate.Builder{}
	builder.Delete(collectionName, messageId)
	_, err := s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}

func (s *storeObject) EditMessage(ctx context.Context, messageId string, newMessage *model.ChatMessage) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()
	obj := marshalModel(arena, newMessage)

	builder := storestate.Builder{}
	err := builder.Modify(collectionName, messageId, []string{contentKey}, pb.ModifyOp_Set, obj.Get(contentKey))
	if err != nil {
		return fmt.Errorf("modify content: %w", err)
	}
	_, err = s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}

func (s *storeObject) ToggleMessageReaction(ctx context.Context, messageId string, emoji string) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	hasReaction, err := s.hasMyReaction(ctx, arena, messageId, emoji)
	if err != nil {
		return fmt.Errorf("check reaction: %w", err)
	}

	builder := storestate.Builder{}

	if hasReaction {
		err = builder.Modify(collectionName, messageId, []string{reactionsKey, emoji}, pb.ModifyOp_Pull, arena.NewString(s.accountService.AccountID()))
		if err != nil {
			return fmt.Errorf("modify content: %w", err)
		}
	} else {
		err = builder.Modify(collectionName, messageId, []string{reactionsKey, emoji}, pb.ModifyOp_AddToSet, arena.NewString(s.accountService.AccountID()))
		if err != nil {
			return fmt.Errorf("modify content: %w", err)
		}
	}

	_, err = s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
		Time:    time.Now(),
	})
	if err != nil {
		return fmt.Errorf("push change: %w", err)
	}
	return nil
}

func (s *storeObject) hasMyReaction(ctx context.Context, arena *anyenc.Arena, messageId string, emoji string) (bool, error) {
	doc, err := s.collection.FindId(ctx, messageId)
	if err != nil {
		return false, fmt.Errorf("find message: %w", err)
	}

	myIdentity := s.accountService.AccountID()
	msg := newMessageWrapper(arena, doc.Value())
	reactions := msg.reactionsToModel()
	if v, ok := reactions.GetReactions()[emoji]; ok {
		if slices.Contains(v.GetIds(), myIdentity) {
			return true, nil
		}
	}
	return false, nil
}

type SubscribeLastMessagesResponse struct {
	Messages  []*model.ChatMessage
	ChatState *model.ChatState
}

func (s *storeObject) SubscribeLastMessages(ctx context.Context, subId string, limit int, asyncInit bool) (*SubscribeLastMessagesResponse, error) {
	txn, err := s.store.NewTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("init read transaction: %w", err)
	}
	defer txn.Commit()

	query := s.collection.Find(nil).Sort(descOrder).Limit(uint(limit))
	messages, err := s.queryMessages(txn.Context(), query)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	// reverse
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].OrderId < messages[j].OrderId
	})

	s.subscription.subscribe(subId)

	if asyncInit {
		var previousOrderId string
		if len(messages) > 0 {
			previousOrderId, err = txn.GetPrevOrderId(messages[0].OrderId)
			if err != nil {
				return nil, fmt.Errorf("get previous order id: %w", err)
			}
		}
		for _, message := range messages {
			s.subscription.add(previousOrderId, message)
			previousOrderId = message.OrderId
		}

		// Force chatState to be sent
		s.subscription.chatStateUpdated = true
		s.subscription.flush()
		return nil, nil
	} else {
		return &SubscribeLastMessagesResponse{
			Messages:  messages,
			ChatState: s.subscription.getChatState(),
		}, nil
	}
}

func (s *storeObject) Unsubscribe(subId string) error {
	s.subscription.unsubscribe(subId)
	return nil
}

func (s *storeObject) TryClose(objectTTL time.Duration) (res bool, err error) {
	if !s.locker.TryLock() {
		return false, nil
	}
	isActive := s.subscription.isActive()
	s.Unlock()

	if isActive {
		return false, nil
	}
	return s.SmartBlock.TryClose(objectTTL)
}

func (s *storeObject) Close() error {
	s.componentCtxCancel()
	return s.SmartBlock.Close()
}

type treeSeenHeadsCollector struct {
	tree objecttree.ObjectTree
}

func newTreeSeenHeadsCollector(tree objecttree.ObjectTree) *treeSeenHeadsCollector {
	return &treeSeenHeadsCollector{
		tree: tree,
	}
}

func (c *treeSeenHeadsCollector) collectSeenHeads(ctx context.Context, afterOrderId string) ([]string, error) {
	var seenHeads []string
	err := c.tree.Storage().GetAfterOrder(ctx, "", func(ctx context.Context, change objecttree.StorageChange) (shouldContinue bool, err error) {
		if change.OrderId >= afterOrderId {
			return false, nil
		}

		seenHeads = slice.DiscardFromSlice(seenHeads, func(id string) bool {
			return slices.Contains(change.PrevIds, id)
		})
		if !slices.Contains(seenHeads, change.Id) {
			seenHeads = append(seenHeads, change.Id)
		}
		return true, nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect seen heads: %w", err)
	}
	return seenHeads, err
}
