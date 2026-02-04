package spaceindex

import (
	"context"
	"errors"
	"fmt"

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

func (s *dsObjectStore) SaveLastIndexedHeadsHash(ctx context.Context, id string, headsHash string) error {
	_, err := s.headsState.UpsertId(ctx, id, query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		if val != nil && val.GetString(headsStateField) == headsHash {
			return val, false, nil
		}
		val.Set(headsStateField, arena.NewString(headsHash))
		return val, true, nil
	}))
	return err
}

// ClearHeadsState efficiently removes all indexed heads hashes by dropping and recreating the collection.
// This causes reindexOutdatedObjects to reindex all objects.
func (s *dsObjectStore) ClearHeadsState(ctx context.Context) error {
	if err := s.headsState.Drop(ctx); err != nil {
		return fmt.Errorf("drop headsState collection: %w", err)
	}
	headsState, err := s.db.Collection(ctx, "headsState")
	if err != nil {
		return fmt.Errorf("reopen headsState collection: %w", err)
	}
	s.headsState = headsState
	return nil
}
