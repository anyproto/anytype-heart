package persistentqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/valyala/fastjson"
)

type FactoryFunc[T Item] func() T

type Storage[T Item] interface {
	Put(item T) error
	Delete(key string) error
	List() ([]T, error)
	Close() error
}

type anystoreStorage[T Item] struct {
	ctx         context.Context
	coll        anystore.Collection
	factoryFunc FactoryFunc[T]
	arena       *anyenc.Arena
}

func NewAnystoreStorage[T Item](ctx context.Context, db anystore.DB, collectionName string, factoryFunc FactoryFunc[T]) (Storage[T], error) {
	coll, err := db.Collection(ctx, collectionName)
	if err != nil {
		return nil, fmt.Errorf("collection: %w", err)
	}

	return &anystoreStorage[T]{
		coll:        coll,
		ctx:         ctx,
		factoryFunc: factoryFunc,
		arena:       &anyenc.Arena{},
	}, nil
}

func (s *anystoreStorage[T]) Put(item T) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	doc, err := anyenc.ParseJson(string(raw))
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	if doc.Get("id") == nil {
		doc.Set("id", s.arena.NewString(item.Key()))
	}

	return s.coll.UpsertOne(s.ctx, doc)
}

func (s *anystoreStorage[T]) Delete(key string) error {
	err := s.coll.DeleteId(s.ctx, key)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil
	}
	return err
}

func (s *anystoreStorage[T]) List() ([]T, error) {
	var items []T
	iter, err := s.coll.Find(nil).Iter(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	buf := make([]byte, 64)
	for iter.Next() {
		item := s.factoryFunc()

		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("get doc: %w", err)
		}

		buf = buf[:0]
		buf = doc.Value().FastJson(&fastjson.Arena{}).MarshalTo(buf)

		err = json.Unmarshal(buf, &item)
		if err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}

		items = append(items, item)
	}
	return items, nil
}

func (s *anystoreStorage[T]) Close() error {
	if s.coll != nil {
		return s.coll.Close()
	}
	return nil
}
