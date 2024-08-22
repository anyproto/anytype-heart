package chatobject

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/gogo/protobuf/jsonpb"
	"github.com/gogo/protobuf/proto"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const collectionName = "chats"
const dataKey = "data"
const creatorKey = "creator"

type StoreObject interface {
	smartblock.SmartBlock

	AddMessage(ctx context.Context, message *model.ChatMessage) (string, error)
	GetMessages(ctx context.Context) ([]*model.ChatMessage, error)
	EditMessage(ctx context.Context, messageId string, newMessage *model.ChatMessage) error
	SubscribeLastMessages(limit int) ([]*model.ChatMessage, int, error)
	Unsubscribe() error
}

type StoreDbProvider interface {
	GetStoreDb() anystore.DB
}

type AccountService interface {
	AccountID() string
}

type storeObject struct {
	smartblock.SmartBlock

	accountService AccountService
	dbProvider     StoreDbProvider
	storeSource    source.Store
	store          *storestate.StoreState
	eventSender    event.Sender

	arenaPool *fastjson.ArenaPool
}

func New(sb smartblock.SmartBlock, accountService AccountService, dbProvider StoreDbProvider, eventSender event.Sender) StoreObject {
	return &storeObject{
		SmartBlock:     sb,
		accountService: accountService,
		dbProvider:     dbProvider,
		arenaPool:      &fastjson.ArenaPool{},
		eventSender:    eventSender,
	}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	err := s.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	stateStore, err := storestate.New(ctx.Ctx, s.Id(), s.dbProvider.GetStoreDb(), ChatHandler{
		chatId:      s.Id(),
		MyIdentity:  s.accountService.AccountID(),
		eventSender: s.eventSender,
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
	err = storeSource.ReadStoreDoc(ctx.Ctx, stateStore)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}

	return nil
}

func (s *storeObject) GetMessages(ctx context.Context) ([]*model.ChatMessage, error) {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	iter, err := coll.Find(nil).Sort("_o.id").Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find iter: %w", err)
	}
	var res []*model.ChatMessage
	unmarshaler := &jsonpb.Unmarshaler{
		AllowUnknownFields: true,
	}
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, errors.Join(iter.Close(), err)
		}

		// TODO Reuse buffer
		raw := doc.Value().Get(dataKey).MarshalTo(nil)

		var message model.ChatMessage
		err = unmarshaler.Unmarshal(bytes.NewReader(raw), &message)
		if err != nil {
			return nil, errors.Join(iter.Close(), fmt.Errorf("unmarshal message: %w", err))
		}
		message.Id = string(doc.Value().GetStringBytes("id"))
		message.OrderId = string(doc.Value().GetStringBytes("_o", "id"))
		message.Creator = string(doc.Value().GetStringBytes(creatorKey))
		res = append(res, &message)
	}
	return res, errors.Join(iter.Close(), err)
}

func (s *storeObject) AddMessage(ctx context.Context, message *model.ChatMessage) (string, error) {
	message = proto.Clone(message).(*model.ChatMessage)
	message.Id = ""
	message.OrderId = ""
	message.Creator = ""

	marshaler := &jsonpb.Marshaler{}
	raw, err := marshaler.MarshalToString(message)
	if err != nil {
		return "", fmt.Errorf("marshal message: %w", err)
	}

	parser := &fastjson.Parser{}
	jsonMessage, err := parser.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse message: %w", err)
	}

	arena := &fastjson.Arena{}
	obj := arena.NewObject()
	obj.Set(dataKey, jsonMessage)
	obj.Set(creatorKey, arena.NewString(s.accountService.AccountID()))

	builder := storestate.Builder{}
	err = builder.Create(collectionName, storestate.IdFromChange, obj)
	if err != nil {
		return "", fmt.Errorf("create chat: %w", err)
	}

	messageId, err := s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
	})
	if err != nil {
		return "", fmt.Errorf("add change: %w", err)
	}
	return messageId, nil
}

func (s *storeObject) EditMessage(ctx context.Context, messageId string, newMessage *model.ChatMessage) error {
	newMessage = proto.Clone(newMessage).(*model.ChatMessage)
	newMessage.Id = ""
	newMessage.OrderId = ""
	newMessage.Creator = ""

	marshaler := &jsonpb.Marshaler{}
	raw, err := marshaler.MarshalToString(newMessage)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	builder := storestate.Builder{}
	err = builder.Modify(collectionName, messageId, []string{dataKey}, pb.ModifyOp_Set, raw)
	if err != nil {
		return fmt.Errorf("modify chat: %w", err)
	}
	_, err = s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
		State:   s.store,
	})
	if err != nil {
		return fmt.Errorf("add change: %w", err)
	}
	return nil
}

func (s *storeObject) SubscribeLastMessages(limit int) ([]*model.ChatMessage, int, error) {
	return nil, 0, nil
}

func (s *storeObject) Unsubscribe() error {
	return nil
}

func (s *storeObject) Close() error {
	// TODO unsubscribe
	return s.SmartBlock.Close()
}
