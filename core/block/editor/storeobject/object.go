package storeobject

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/valyala/fastjson"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/pb"
)

const collectionName = "chats"

type StoreObject interface {
	smartblock.SmartBlock

	AddMessage(ctx context.Context, message string) (string, error)
	GetMessages(ctx context.Context) ([]string, error)
	EditMessage(ctx context.Context, messageId string, newText string) error
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
}

func New(sb smartblock.SmartBlock, accountService AccountService, dbProvider StoreDbProvider) StoreObject {
	return &storeObject{SmartBlock: sb, accountService: accountService, dbProvider: dbProvider}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	err := s.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	stateStore, err := storestate.New(ctx.Ctx, s.Id(), s.dbProvider.GetStoreDb(), ChatHandler{
		MyIdentity: s.accountService.AccountID(),
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

func (s *storeObject) GetMessages(ctx context.Context) ([]string, error) {
	coll, err := s.store.Collection(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("get collection: %w", err)
	}
	iter, err := coll.Find(nil).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("find iter: %w", err)
	}
	var res []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, errors.Join(iter.Close(), err)
		}
		v := map[string]any{}
		err = doc.Decode(&v)
		if err != nil {
			return nil, errors.Join(iter.Close(), err)
		}

		out, _ := json.Marshal(v)
		res = append(res, string(out))
		// res = append(res, v["text"])
	}
	return res, errors.Join(iter.Close(), err)
}

func (s *storeObject) AddMessage(ctx context.Context, text string) (string, error) {
	builder := storestate.Builder{}
	err := builder.Create(collectionName, storestate.IdFromChange, map[string]string{
		"text":   text,
		"author": s.accountService.AccountID(),
	})
	if err != nil {
		return "", fmt.Errorf("create chat: %w", err)
	}

	messageId, err := s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
	})
	if err != nil {
		return "", fmt.Errorf("add change: %w", err)
	}
	return messageId, nil
}

func (s *storeObject) EditMessage(ctx context.Context, messageId string, newText string) error {
	arena := &fastjson.Arena{}

	builder := storestate.Builder{}
	err := builder.Modify("chats", messageId, []string{"text"}, pb.ModifyOp_Set, arena.NewString(newText))
	if err != nil {
		return fmt.Errorf("modify chat: %w", err)
	}
	_, err = s.storeSource.PushStoreChange(ctx, source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
	})
	if err != nil {
		return fmt.Errorf("add change: %w", err)
	}
	return nil
}
