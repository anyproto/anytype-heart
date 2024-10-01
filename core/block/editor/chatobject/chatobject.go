package chatobject

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/query"
	"github.com/valyala/fastjson"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceobjects"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	collectionName = "chats"
	descOrder      = "-_o.id"
)

type StoreObject interface {
	smartblock.SmartBlock

	AddMessage(ctx context.Context, sessionCtx session.Context, message *model.ChatMessage) (string, error)
	GetMessages(ctx context.Context, beforeOrderId string, limit int) ([]*model.ChatMessage, error)
	GetMessagesByIds(ctx context.Context, messageIds []string) ([]*model.ChatMessage, error)
	EditMessage(ctx context.Context, messageId string, newMessage *model.ChatMessage) error
	ToggleMessageReaction(ctx context.Context, messageId string, emoji string) error
	DeleteMessage(ctx context.Context, messageId string) error
	SubscribeLastMessages(ctx context.Context, limit int) ([]*model.ChatMessage, int, error)
	Unsubscribe() error
}

type AccountService interface {
	AccountID() string
}

type storeObject struct {
	smartblock.SmartBlock
	locker smartblock.Locker

	accountService AccountService
	storeSource    source.Store
	store          *storestate.StoreState
	eventSender    event.Sender
	subscription   *subscription
	spaceObjects   spaceobjects.Store

	arenaPool *fastjson.ArenaPool
}

func New(sb smartblock.SmartBlock, accountService AccountService, spaceObjects spaceobjects.Store, eventSender event.Sender) StoreObject {
	return &storeObject{
		SmartBlock:     sb,
		locker:         sb.(smartblock.Locker),
		accountService: accountService,
		spaceObjects:   spaceObjects,
		arenaPool:      &fastjson.ArenaPool{},
		eventSender:    eventSender,
	}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	err := s.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	s.subscription = newSubscription(s.Id(), s.eventSender)

	stateStore, err := storestate.New(ctx.Ctx, s.Id(), s.spaceObjects.GetDb(), ChatHandler{
		chatId:       s.Id(),
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

	return nil
}

func (s *storeObject) onUpdate() {
	s.subscription.flush()
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

func (s *storeObject) GetMessages(ctx context.Context, beforeOrderId string, limit int) ([]*model.ChatMessage, error) {
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	var msgs []*model.ChatMessage
	if beforeOrderId != "" {
		qry := coll.Find(query.Key{Path: []string{orderKey, "id"}, Filter: query.NewComp(query.CompOpLt, beforeOrderId)}).Sort(descOrder).Limit(uint(limit))
		msgs, err = s.queryMessages(ctx, qry)
		if err != nil {
			return nil, fmt.Errorf("query messages: %w", err)
		}
	} else {
		qry := coll.Find(nil).Sort(descOrder).Limit(uint(limit))
		msgs, err = s.queryMessages(ctx, qry)
		if err != nil {
			return nil, fmt.Errorf("query messages: %w", err)
		}
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
	var res []*model.ChatMessage
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, errors.Join(iter.Close(), err)
		}

		message := newMessageWrapper(arena, doc.Value()).toModel()
		res = append(res, message)
	}
	return res, errors.Join(iter.Close(), err)
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

func (s *storeObject) hasMyReaction(ctx context.Context, arena *fastjson.Arena, messageId string, emoji string) (bool, error) {
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

	var firstOrderId string
	if len(messages) > 0 {
		firstOrderId = messages[0].OrderId
	}
	s.subscription.subscribe(firstOrderId)

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
	return s.SmartBlock.Close()
}
