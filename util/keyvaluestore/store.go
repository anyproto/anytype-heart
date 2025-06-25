package keyvaluestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
)

type KeyValue[T any] struct {
	Key   string
	Value T
}

const valueKey = "_v"

// Store is a simple generic key-value store backed by any-store
type Store[T any] interface {
	Get(ctx context.Context, key string) (T, error)
	Set(ctx context.Context, key string, value T) error
	Delete(ctx context.Context, key string) error
	Has(ctx context.Context, key string) (bool, error)
	ListAllValues(ctx context.Context) ([]T, error)

	Iterator(ctx context.Context) *Iterator[T]

	// Proxies for any-store transactions
	ReadTx(ctx context.Context) (anystore.ReadTx, error)
	WriteTx(ctx context.Context) (anystore.WriteTx, error)
}

type store[T any] struct {
	coll      anystore.Collection
	arenaPool *anyenc.ArenaPool

	marshaller   func(T) ([]byte, error)
	unmarshaller func([]byte) (T, error)
}

func New[T any](
	db anystore.DB,
	collectionName string,
	marshaller func(T) ([]byte, error),
	unmarshaller func([]byte) (T, error),
) (Store[T], error) {

	coll, err := db.Collection(context.Background(), collectionName)
	if err != nil {
		return nil, fmt.Errorf("init collection: %w", err)
	}

	return NewFromCollection(coll, marshaller, unmarshaller), nil
}

func NewFromCollection[T any](
	coll anystore.Collection,
	marshaller func(T) ([]byte, error),
	unmarshaller func([]byte) (T, error),
) Store[T] {
	return &store[T]{
		coll:         coll,
		marshaller:   marshaller,
		unmarshaller: unmarshaller,
		arenaPool:    &anyenc.ArenaPool{},
	}
}

// NewJson creates a new Store that marshals and unmarshals values as JSON
func NewJson[T any](
	db anystore.DB,
	collectionName string,
) (Store[T], error) {
	return New[T](db, collectionName, JsonMarshal[T], JsonUnmarshal[T])
}

func NewJsonFromCollection[T any](coll anystore.Collection) Store[T] {
	return NewFromCollection[T](coll, JsonMarshal[T], JsonUnmarshal[T])
}

func (s *store[T]) ReadTx(ctx context.Context) (anystore.ReadTx, error) {
	return s.coll.ReadTx(ctx)
}

func (s *store[T]) WriteTx(ctx context.Context) (anystore.WriteTx, error) {
	return s.coll.WriteTx(ctx)
}

func (s *store[T]) Get(ctx context.Context, key string) (T, error) {
	var res T
	doc, err := s.coll.FindId(ctx, key)
	if err != nil {
		return res, err
	}

	raw := doc.Value().GetBytes(valueKey)
	if raw == nil {
		return res, anystore.ErrDocNotFound
	}

	return s.unmarshaller(raw)
}

func (s *store[T]) Iterator(ctx context.Context) *Iterator[T] {
	return &Iterator[T]{
		err:          nil,
		coll:         s.coll,
		unmarshaller: s.unmarshaller,
		ctx:          ctx,
	}
}

func (s *store[T]) ListAllValues(ctx context.Context) ([]T, error) {
	var res []T
	it := s.Iterator(ctx)
	for _, v := range it.All() {
		res = append(res, v)
	}
	return res, it.Err()
}

func (s *store[T]) Has(ctx context.Context, key string) (bool, error) {
	_, err := s.coll.FindId(ctx, key)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (s *store[T]) Set(ctx context.Context, key string, value T) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	raw, err := s.marshaller(value)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	doc := arena.NewObject()
	doc.Set("id", arena.NewString(key))
	doc.Set(valueKey, arena.NewBinary(raw))

	return s.coll.UpsertOne(ctx, doc)
}

func (s *store[T]) Delete(ctx context.Context, key string) error {
	err := s.coll.DeleteId(ctx, key)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil
	}
	return err
}

type Iterator[T any] struct {
	err          error
	ctx          context.Context
	coll         anystore.Collection
	unmarshaller func([]byte) (T, error)
}

func (it *Iterator[T]) All() iter.Seq2[string, T] {
	return func(yield func(string, T) bool) {
		iter, err := it.coll.Find(nil).Iter(it.ctx)
		if err != nil {
			it.err = fmt.Errorf("init iter: %w", err)
			return
		}
		defer iter.Close()

		for iter.Next() {
			doc, err := iter.Doc()
			if err != nil {
				it.err = fmt.Errorf("get document: %w", err)
				return
			}
			raw := doc.Value().GetBytes(valueKey)

			val, err := it.unmarshaller(raw)
			if err != nil {
				it.err = fmt.Errorf("unmarshal: %w", err)
				return
			}

			yield(doc.Value().GetString("id"), val)
		}
	}
}

func (it *Iterator[T]) Err() error {
	return it.err
}

func JsonMarshal[T any](val T) ([]byte, error) {
	return json.Marshal(val)
}

func JsonUnmarshal[T any](data []byte) (T, error) {
	var val T
	err := json.Unmarshal(data, &val)
	return val, err
}

func BytesMarshal(val []byte) ([]byte, error) {
	return val, nil
}

func BytesUnmarshal(data []byte) ([]byte, error) {
	return data, nil
}

func StringMarshal(val string) ([]byte, error) {
	return []byte(val), nil
}

func StringUnmarshal(data []byte) (string, error) {
	return string(data), nil
}
