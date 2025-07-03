package spaceindex

import (
	"context"
	"errors"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
)

const headsStateField = "h"

// GetLastIndexedHeadsHash return empty hash without error if record was not found
func (s *dsObjectStore) GetLastIndexedHeadsHash(ctx context.Context, id string) (headsHash string, err error) {
	doc, err := s.headsState.FindId(ctx, id)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(doc.Value().GetStringBytes(headsStateField)), nil
}

func (s *dsObjectStore) IterateLastIndexedHeadsHash(ctx context.Context, fn func(id string, headsHash string) (stop bool, err error)) error {
	iter, err := s.headsState.Find(nil).Iter(ctx)
	if err != nil {
		return err
	}
	defer iter.Close()
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return err
		}
		id := doc.Value().GetString("id")
		headsHash := doc.Value().GetString(headsStateField)
		stop, err := fn(id, headsHash)
		if stop || err != nil {
			return err
		}
	}

	return iter.Err()
}

func (s *dsObjectStore) SaveLastIndexedHeadsHash(ctx context.Context, id string, headsHash string) error {
	_, err := s.headsState.UpsertId(ctx, id, query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		val.Set(headsStateField, arena.NewString(headsHash))
		return val, true, nil
	}))
	return err
}
