package chatobject

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/anystoredebug"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logger.NewNamed("common.editor.chatobject")

const (
	collectionName = "chats"
	descOrder      = "-_o.id"
	ascOrder       = "_o.id"
	descAdded      = "-a"
)

type StoreObject interface {
	smartblock.SmartBlock
	anystoredebug.AnystoreDebug

	AddMessage(ctx context.Context, sessionCtx session.Context, message *model.ChatMessage) (string, error)
	GetMessages(ctx context.Context, req GetMessagesRequest) ([]*model.ChatMessage, *model.ChatState, error)
	GetMessagesByIds(ctx context.Context, messageIds []string) ([]*model.ChatMessage, error)
	EditMessage(ctx context.Context, messageId string, newMessage *model.ChatMessage) error
	ToggleMessageReaction(ctx context.Context, messageId string, emoji string) error
	DeleteMessage(ctx context.Context, messageId string) error
	SubscribeLastMessages(ctx context.Context, limit int) ([]*model.ChatMessage, int, error)
	MarkReadMessages(ctx context.Context, afterOrderId string, beforeOrderId string, lastAddedMessageTimestamp int64) error
	Unsubscribe() error
}

type GetMessagesRequest struct {
	AfterOrderId  string
	BeforeOrderId string
	Limit         int
}

type AccountService interface {
	AccountID() string
}

type storeObject struct {
	anystoredebug.AnystoreDebug
	smartblock.SmartBlock
	locker smartblock.Locker

	accountService AccountService
	storeSource    source.Store
	store          *storestate.StoreState
	eventSender    event.Sender
	subscription   *subscription
	crdtDb         anystore.DB

	arenaPool          *anyenc.ArenaPool
	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
}

func New(sb smartblock.SmartBlock, accountService AccountService, eventSender event.Sender, crdtDb anystore.DB) StoreObject {
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
	}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	storeSource, ok := ctx.Source.(source.Store)
	if !ok {
		return fmt.Errorf("source is not a store")
	}

	storeSource.SetDiffManagerOnRemoveHook(s.markReadMessages)
	err := s.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	s.subscription = newSubscription(s.SpaceID(), s.Id(), s.eventSender)

	stateStore, err := storestate.New(ctx.Ctx, s.Id(), s.crdtDb, ChatHandler{
		subscription:    s.subscription,
		currentIdentity: s.accountService.AccountID(),
	})

	if err != nil {
		return fmt.Errorf("create state store: %w", err)
	}
	s.store = stateStore

	s.storeSource = storeSource
	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, s.onUpdate)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}

	s.AnystoreDebug = anystoredebug.New(s.SmartBlock, stateStore)

	return nil
}

func (s *storeObject) onUpdate() {
	_ = s.subscription.flush()
}

// initialChatState returns the initial chat state for the chat object from the DB
func (s *storeObject) initialChatState() (*model.ChatState, error) {
	coll, err := s.store.Collection(s.componentCtx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}

	txn, err := coll.ReadTx(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	defer func() {
		errCommit := txn.Commit()
		if errCommit != nil {
			log.With(zap.Error(errCommit)).Error("read tx commit error")
		}
	}()

	ctx := txn.Context()

	unreadQuery := coll.Find(query.Key{Path: []string{readKey}, Filter: query.NewComp(query.CompOpEq, false)}).Sort(ascOrder)
	iter, err := unreadQuery.Limit(1).Iter(ctx)
	var oldestOrderId string
	defer iter.Close()
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		oldestOrderId = doc.Value().GetObject(orderKey).Get("id").GetString()
	}

	count, err := unreadQuery.Count(ctx)
	if err != nil {
		return nil, fmt.Errorf("update messages: %w", err)
	}

	lastAddedDate := coll.Find(query.All{}).Sort(descAdded).Limit(1)
	iter, err = lastAddedDate.Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find last added date: %w", err)
	}
	defer iter.Close()
	var lastAdded int
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		lastAdded = doc.Value().GetInt(addedKey)
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
func (s *storeObject) markReadMessages(ids []string) {
	if len(ids) == 0 {
		return
	}
	coll, err := s.store.Collection(s.componentCtx, collectionName)
	if err != nil {
		log.With(zap.Error(err)).Error("markReadMessages: get collection")
		return
	}
	txn, err := coll.WriteTx(s.componentCtx)
	if err != nil {
		log.With(zap.Error(err)).Error("markReadMessages: start write tx")
		return
	}
	ctx := txn.Context()
	idsModified := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == s.Id() {
			// skip tree root
			continue
		}
		res, err := coll.UpdateId(ctx, id, query.MustParseModifier(`{"$set":{"`+readKey+`":true}}`))
		if err != nil {
			log.With(zap.Error(err)).With(zap.String("id", id)).With(zap.String("chatId", s.Id())).Error("markReadMessages: update message")
			continue
		}
		if res.Modified > 0 {
			idsModified = append(idsModified, id)
		}
	}
	err = txn.Commit()
	if err != nil {
		log.With(zap.Error(err)).Error("markReadMessages: commit")
		return
	}
	log.Debug(fmt.Sprintf("markReadMessages: %d/%d messages marked as read", len(idsModified), len(ids)))
	if len(idsModified) > 0 {
		// it doesn't work within the same transaction
		// query the new oldest unread message's orderId
		iter, err := coll.Find(
			query.Key{Path: []string{readKey}, Filter: query.NewComp(query.CompOpEq, false)},
		).Sort(ascOrder).
			Limit(1).
			Iter(s.componentCtx)
		if err != nil {
			log.With(zap.Error(err)).Error("markReadMessages: failed to find oldest unread message")
		}
		defer iter.Close()
		var newOldestOrderId string
		if iter.Next() {
			val, err := iter.Doc()
			if err != nil {
				log.With(zap.Error(err)).Error("markReadMessages: failed to get oldest unread message")
			}
			if val != nil {
				newOldestOrderId = val.Value().GetObject(orderKey).Get("id").GetString()
			}
		}
		log.Debug(fmt.Sprintf("markReadMessages: new oldest unread message: %s", newOldestOrderId))
		s.subscription.chatState.Messages.OldestOrderId = newOldestOrderId
		s.subscription.updateReadStatus(idsModified, true)
		s.onUpdate()
	}
}

func (s *storeObject) MarkReadMessages(ctx context.Context, afterOrderId, beforeOrderId string, lastAddedMessageTimestamp int64) error {
	// 1. select all messages with orderId < beforeOrderId and addedTime < lastDbState
	// 2. use the last(by orderId) message id as lastHead
	// 3. update the MarkSeenHeads
	// 2. mark messages as read in the DB

	msg, err := s.GetLastAddedMessageInOrderRange(ctx, afterOrderId, beforeOrderId, lastAddedMessageTimestamp)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	// mark the whole tree as seen from the current message
	s.storeSource.MarkSeenHeads([]string{msg.Id})
	return nil
}

func (s *storeObject) GetLastAddedMessageInOrderRange(ctx context.Context, afterOrderId, beforeOrderId string, lastAddedMessageTimestamp int64) (*model.ChatMessage, error) {
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}

	if lastAddedMessageTimestamp < 0 {
		// todo: remove this
		// for testing purposes
		lastAddedMessageTimestamp = math.MaxInt64
	}
	iter, err := coll.Find(
		query.And{
			query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpGte, afterOrderId)},
			query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpLte, beforeOrderId)},
			query.Key{Path: []string{addedKey}, Filter: query.NewComp(query.CompOpLte, lastAddedMessageTimestamp)},
		},
	).Sort(descAdded).
		Limit(1).
		Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find id: %w", err)
	}
	defer iter.Close()
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}
		msg := newMessageWrapper(nil, doc.Value()).toModel()
		return msg, nil
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("iter error: %w", err)
	}

	return nil, anystore.ErrDocNotFound
}

func (s *storeObject) GetMessagesByIds(ctx context.Context, messageIds []string) ([]*model.ChatMessage, error) {
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	txn, err := coll.ReadTx(ctx)
	if err != nil {
		return nil, fmt.Errorf("start read tx: %w", err)
	}
	messages := make([]*model.ChatMessage, 0, len(messageIds))
	for _, messageId := range messageIds {
		obj, err := coll.FindId(txn.Context(), messageId)
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

func (s *storeObject) GetMessages(ctx context.Context, req GetMessagesRequest) ([]*model.ChatMessage, *model.ChatState, error) {
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, nil, fmt.Errorf("get collection: %w", err)
	}
	var qry anystore.Query
	if req.AfterOrderId != "" {
		qry = coll.Find(query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpGt, req.AfterOrderId)}).Sort(ascOrder).Limit(uint(req.Limit))
	} else if req.BeforeOrderId != "" {
		qry = coll.Find(query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpLt, req.BeforeOrderId)}).Sort(descOrder).Limit(uint(req.Limit))
	} else {
		qry = coll.Find(nil).Sort(descOrder).Limit(uint(req.Limit))
	}
	// make sure we flush all the pending message updates first
	chatState := s.subscription.flush()
	// todo here is possible race if new messages are added between the flush and the query
	msgs, err := s.queryMessages(ctx, qry)
	if err != nil {
		return nil, nil, fmt.Errorf("query messages: %w", err)
	}
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].OrderId < msgs[j].OrderId
	})

	return msgs, chatState, nil
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
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return false, fmt.Errorf("get collection: %w", err)
	}
	doc, err := coll.FindId(ctx, messageId)
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

func (s *storeObject) SubscribeLastMessages(ctx context.Context, limit int) ([]*model.ChatMessage, int, error) {
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, 0, fmt.Errorf("get collection: %w", err)
	}
	query := coll.Find(nil).Sort(descOrder).Limit(uint(limit))
	messages, err := s.queryMessages(ctx, query)
	if err != nil {
		return nil, 0, fmt.Errorf("query messages: %w", err)
	}
	// reverse
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].OrderId < messages[j].OrderId
	})

	if s.subscription.enable() {
		s.subscription.chatState, err = s.initialChatState()
		if err != nil {
			return nil, 0, fmt.Errorf("failed to fetch initial chat state: %w", err)
		}
	}

	return messages, 0, nil
}

func (s *storeObject) Unsubscribe() error {
	s.subscription.close()
	return nil
}

func (s *storeObject) TryClose(objectTTL time.Duration) (res bool, err error) {
	if !s.locker.TryLock() {
		return false, nil
	}
	isActive := s.subscription.enabled
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
