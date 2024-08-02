package storeobject

import (
	"context"
	"errors"
	"fmt"

	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
)

type StoreObject interface {
	smartblock.SmartBlock

	AddMessage(ctx context.Context, message string) error
	GetMessages(ctx context.Context) ([]string, error)
}

type storeObject struct {
	smartblock.SmartBlock

	storeSource source.Store
	store       *storestate.StoreState
}

func New(sb smartblock.SmartBlock) StoreObject {
	return &storeObject{SmartBlock: sb}
}

func (s *storeObject) Init(ctx *smartblock.InitContext) error {
	err := s.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}

	storeSource, ok := ctx.Source.(source.Store)
	if !ok {
		return fmt.Errorf("source is not a store")
	}
	s.storeSource = storeSource
	err = storeSource.ReadStoreDoc(ctx.Ctx)
	if err != nil {
		return fmt.Errorf("read store doc: %w", err)
	}
	s.store = storeSource.GetStore()

	return nil
}

func (s *storeObject) GetMessages(ctx context.Context) ([]string, error) {
	coll, err := s.store.Collection(ctx, "chats")
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
		res = append(res, v["text"].(string))
		// res = append(res, v["text"])
	}
	return res, errors.Join(iter.Close(), err)
}

func (s *storeObject) AddMessage(ctx context.Context, text string) error {
	tx, err := s.store.NewTx(ctx)
	if err != nil {
		return fmt.Errorf("new tx: %w", err)
	}

	builder := storestate.Builder{}
	err = builder.Create("chats", bson.NewObjectId().Hex(), map[string]string{
		"text":      text,
		"someField": "additional",
	})
	if err != nil {
		return fmt.Errorf("create chat: %w", err)
	}

	err = tx.ApplyChangeSet(storestate.ChangeSet{
		Changes: builder.ChangeSet,
	})
	if err != nil {
		return fmt.Errorf("apply change set: %w", err)
	}

	_, err = s.storeSource.PushStoreChange(source.PushStoreChangeParams{
		Changes: builder.ChangeSet,
	})
	if err != nil {
		return fmt.Errorf("push store change: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}

	return nil
}
