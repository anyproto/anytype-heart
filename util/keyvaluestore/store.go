package keyvaluestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
)

const valueKey = "_v"

// TODO Change to any-store or domain error
var ErrNotFound = fmt.Errorf("not found")

// Store is a simple generic key-value store backed by any-store
type Store[T any] interface {
	Get(key string) (T, error)
	Set(key string, value T) error
	Delete(key string) error
	Has(key string) (bool, error)
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

	return &store[T]{
		coll:         coll,
		marshaller:   marshaller,
		unmarshaller: unmarshaller,
		arenaPool:    &anyenc.ArenaPool{},
	}, nil
}

// NewJson creates a new Store that marshals and unmarshals values as JSON
func NewJson[T any](
	db anystore.DB,
	collectionName string,
) (Store[T], error) {
	return New[T](db, collectionName, JsonMarshal[T], JsonUnmarshal[T])
}

func (s *store[T]) Get(key string) (T, error) {
	var res T
	doc, err := s.coll.FindId(context.Background(), key)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return res, ErrNotFound
	}
	if err != nil {
		return res, err
	}

	raw := doc.Value().GetBytes(valueKey)
	if raw == nil {
		return res, ErrNotFound
	}

	return s.unmarshaller(raw)
}

func (s *store[T]) Has(key string) (bool, error) {
	_, err := s.coll.FindId(context.Background(), key)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func (s *store[T]) Set(key string, value T) error {
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

	return s.coll.UpsertOne(context.Background(), doc)
}

func (s *store[T]) Delete(key string) error {
	err := s.coll.DeleteId(context.Background(), key)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return nil
	}
	return err
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
