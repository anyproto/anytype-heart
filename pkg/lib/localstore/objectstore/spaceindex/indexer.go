package spaceindex

import (
	"context"
	"errors"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/query"
	"github.com/valyala/fastjson"
)

const headsStateField = "h"

// GetLastIndexedHeadsHash return empty hash without error if record was not found
func (s *dsObjectStore) GetLastIndexedHeadsHash(ctx context.Context, id string) (headsHash string, err error) {
	doc, err := s.headsState.FindId(ctx, id)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return "", nil
	}
	return string(doc.Value().GetStringBytes(headsStateField)), nil
}

func (s *dsObjectStore) SaveLastIndexedHeadsHash(ctx context.Context, id string, headsHash string) error {
	_, err := s.headsState.UpsertId(ctx, id, query.ModifyFunc(func(arena *fastjson.Arena, val *fastjson.Value) (*fastjson.Value, bool, error) {
		val.Set(headsStateField, arena.NewString(headsHash))
		return val, true, nil
	}))
	return err
}
