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
	collectionName = "chats"
	descOrder      = "-_o.id"
	ascOrder       = "_o.id"
)

var log = logging.Logger("core.block.editor.chatobject").Desugar()

type StoreObject interface {
	smartblock.SmartBlock
	anystoredebug.AnystoreDebug

	AddMessage(ctx context.Context, sessionCtx session.Context, message *model.ChatMessage) (string, error)
	GetMessages(ctx context.Context, req GetMessagesRequest) ([]*model.ChatMessage, error)
	GetMessagesByIds(ctx context.Context, messageIds []string) ([]*model.ChatMessage, error)
	EditMessage(ctx context.Context, messageId string, newMessage *model.ChatMessage) error
	ToggleMessageReaction(ctx context.Context, messageId string, emoji string) error
	DeleteMessage(ctx context.Context, messageId string) error
	SubscribeLastMessages(ctx context.Context, subId string, limit int, asyncInit bool) ([]*model.ChatMessage, int, error)
	MarkSeenHeads(heads []string)
	Unsubscribe(subId string) error
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
	spaceIndex     spaceindex.Store

	arenaPool *anyenc.ArenaPool
}

func New(sb smartblock.SmartBlock, accountService AccountService, eventSender event.Sender, crdtDb anystore.DB, spaceIndex spaceindex.Store) StoreObject {
	return &storeObject{
		SmartBlock:     sb,
		locker:         sb.(smartblock.Locker),
		accountService: accountService,
		arenaPool:      &anyenc.ArenaPool{},
		eventSender:    eventSender,
		crdtDb:         crdtDb,
		spaceIndex:     spaceIndex,
	}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	err := s.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	s.subscription = newSubscription(s.SpaceID(), s.Id(), s.eventSender, s.spaceIndex)

	stateStore, err := storestate.New(ctx.Ctx, s.Id(), s.crdtDb, ChatHandler{
		subscription: s.subscription,
	})
	if err != nil {
		return fmt.Errorf("create state store: %w", err)
	}
	s.store = stateStore

	storeSource, ok := ctx.Source.(source.Store)
	if !ok {
		return fmt.Errorf("source is not a store")
	}
	s.storeSource = storeSource
	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore, s.onUpdate)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}

	s.AnystoreDebug = anystoredebug.New(s.SmartBlock, stateStore)

	return nil
}

func (s *storeObject) onUpdate() {
	s.subscription.flush()
}

func (s *storeObject) MarkSeenHeads(heads []string) {
	s.storeSource.MarkSeenHeads(heads)
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

func (s *storeObject) GetMessages(ctx context.Context, req GetMessagesRequest) ([]*model.ChatMessage, error) {
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	var qry anystore.Query
	if req.AfterOrderId != "" {
		qry = coll.Find(query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpGt, req.AfterOrderId)}).Sort(ascOrder).Limit(uint(req.Limit))
	} else if req.BeforeOrderId != "" {
		qry = coll.Find(query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpLt, req.BeforeOrderId)}).Sort(descOrder).Limit(uint(req.Limit))
	} else {
		qry = coll.Find(nil).Sort(descOrder).Limit(uint(req.Limit))
	}
	msgs, err := s.queryMessages(ctx, qry)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	sort.Slice(msgs, func(i, j int) bool {
		return msgs[i].OrderId < msgs[j].OrderId
	})
	return msgs, nil
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

func (s *storeObject) SubscribeLastMessages(ctx context.Context, subId string, limit int, asyncInit bool) ([]*model.ChatMessage, int, error) {
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, 0, fmt.Errorf("get collection: %w", err)
	}

	txn, err := s.store.NewTx(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("init read transaction: %w", err)
	}
	defer txn.Commit()

	query := coll.Find(nil).Sort(descOrder).Limit(uint(limit))
	messages, err := s.queryMessages(txn.Context(), query)
	if err != nil {
		return nil, 0, fmt.Errorf("query messages: %w", err)
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
				return nil, 0, fmt.Errorf("get previous order id: %w", err)
			}
		}

		for _, message := range messages {
			s.subscription.add(previousOrderId, message)
			previousOrderId = message.OrderId
		}
		s.subscription.flush()
		return nil, 0, nil
	} else {
		return messages, 0, nil
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
	return s.SmartBlock.Close()
}
