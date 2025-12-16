package filequeue

import (
	"context"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
)

type marshalFunc[T any] func(arena *anyenc.Arena, val T) *anyenc.Value
type unmarshalFunc[T any] func(v *anyenc.Value) (T, error)

type Storage[T any] struct {
	arena     *anyenc.Arena
	coll      anystore.Collection
	marshal   marshalFunc[T]
	unmarshal unmarshalFunc[T]
}

func NewStorage[T any](coll anystore.Collection, marshal marshalFunc[T], unmarshal unmarshalFunc[T]) *Storage[T] {
	return &Storage[T]{
		arena:     &anyenc.Arena{},
		coll:      coll,
		marshal:   marshal,
		unmarshal: unmarshal,
	}
}

func (s *Storage[T]) get(ctx context.Context, objectId string) (T, error) {
	doc, err := s.coll.FindId(ctx, objectId)
	if errors.Is(err, anystore.ErrDocNotFound) {
		var defVal T
		return defVal, ErrNotFound
	}
	if err != nil {
		var defVal T
		return defVal, err
	}

	return s.unmarshal(doc.Value())
}

func (s *Storage[T]) set(ctx context.Context, objectId string, file T) error {
	defer s.arena.Reset()

	val := s.marshal(s.arena, file)
	val.Set("id", s.arena.NewString(objectId))
	return s.coll.UpsertOne(ctx, val)
}

func (s *Storage[T]) delete(ctx context.Context, objectId string) error {
	return s.coll.DeleteId(ctx, objectId)
}

func (s *Storage[T]) query(ctx context.Context, filter query.Filter, order query.Sort, inMemoryFilter func(T) bool) (T, error) {
	var defVal T

	var sortArgs []any
	if order != nil {
		sortArgs = []any{order}
	}

	// Unfortunately, we can't use limit as we need to check row locks on the application level
	// TODO Maybe query items by some batch, for example 10 items at once
	iter, err := s.coll.Find(filter).Sort(sortArgs...).Iter(ctx)
	if err != nil {
		return defVal, fmt.Errorf("iter: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return defVal, fmt.Errorf("read doc: %w", err)
		}

		val, err := s.unmarshal(doc.Value())
		if err != nil {
			return defVal, fmt.Errorf("unmarshal: %w", err)
		}

		if inMemoryFilter(val) {
			return val, nil
		}
	}

	return defVal, ErrNoRows
}

func (s *Storage[T]) listAll(ctx context.Context) ([]T, error) {
	iter, err := s.coll.Find(nil).Iter(ctx)
	if err != nil {
		return nil, fmt.Errorf("iter: %w", err)
	}
	defer iter.Close()

	var res []T
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("read doc: %w", err)
		}

		val, err := s.unmarshal(doc.Value())
		if err != nil {
			return nil, fmt.Errorf("unmarshal: %w", err)
		}
		res = append(res, val)
	}
	return res, nil
}
