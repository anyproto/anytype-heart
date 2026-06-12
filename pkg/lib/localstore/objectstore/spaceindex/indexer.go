package spaceindex

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/cespare/xxhash/v2"
)

const headsStateField = "h"
const ftQueueCtrField = "ftQueueCtr" // FT queue counter for crash recovery consistency
const lastReconciledLinksField = "lr"

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

// SaveLastIndexedHeadsHashWithFtQueueCtr saves the heads hash along with the FT queue counter.
// The ftQueueCtr is used for crash recovery: if the common DB (FT queue) doesn't flush before crash,
// we can detect objects with ftQueueCtr > persisted counter and re-add them to the queue.
func (s *dsObjectStore) SaveLastIndexedHeadsHashWithFtQueueCtr(ctx context.Context, id string, headsHash string, ftQueueCtr uint64) error {
	_, err := s.headsState.UpsertId(ctx, id, query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		existingHash := ""
		existingCtr := uint64(0)
		if val != nil {
			existingHash = val.GetString(headsStateField)
			existingCtr = uint64(val.GetFloat64(ftQueueCtrField))
		}

		// Skip if nothing changed
		if existingHash == headsHash && existingCtr == ftQueueCtr {
			return val, false, nil
		}

		val.Set(headsStateField, arena.NewString(headsHash))
		if ftQueueCtr > 0 {
			val.Set(ftQueueCtrField, arena.NewNumberFloat64(float64(ftQueueCtr)))
		}
		return val, true, nil
	}))
	return err
}

// HashLinksList returns a stable fingerprint of a set of linked object ids, insensitive to
// order and duplicates (the link-derived details reconcile is a set diff, so reordering
// favorites must not look like a change). Non-empty even for an empty list, so an absent
// marker is distinguishable from a reconciled-empty one.
func HashLinksList(ids []string) string {
	uniq := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			uniq = append(uniq, id)
		}
	}
	sort.Strings(uniq)
	digest := xxhash.New()
	for _, id := range uniq {
		_, _ = digest.WriteString(id)
		_, _ = digest.WriteString("/")
	}
	return strconv.FormatUint(digest.Sum64(), 16)
}

// GetLastReconciledLinksHash returns the links fingerprint persisted by the last completed
// link-derived details reconcile of the object (Home/Archive), or empty if none was recorded.
func (s *dsObjectStore) GetLastReconciledLinksHash(ctx context.Context, id string) (hash string, err error) {
	doc, err := s.headsState.FindId(ctx, id)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(doc.Value().GetStringBytes(lastReconciledLinksField)), nil
}

// SaveLastReconciledLinksHash records that the link-derived details (isFavorite/isArchived)
// were fully reconciled against the links fingerprinted by hash (see HashLinksList).
func (s *dsObjectStore) SaveLastReconciledLinksHash(ctx context.Context, id string, hash string) error {
	_, err := s.headsState.UpsertId(ctx, id, query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		if val != nil && val.GetString(lastReconciledLinksField) == hash {
			return val, false, nil
		}
		val.Set(lastReconciledLinksField, arena.NewString(hash))
		return val, true, nil
	}))
	return err
}

// HeadsStateEntry represents an entry in the headsState collection
type HeadsStateEntry struct {
	ObjectID   string
	HeadsHash  string
	FTQueueCtr uint64
}

// GetHeadsWithFtQueueCtrGreaterThan returns all headsState entries where ftQueueCtr > threshold.
// Used for crash recovery to identify objects that may have been added to FT queue
// but the queue write wasn't persisted due to a crash.
func (s *dsObjectStore) GetHeadsWithFtQueueCtrGreaterThan(ctx context.Context, threshold uint64) ([]HeadsStateEntry, error) {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	// Build filter: ftQueueCtr > threshold
	filter := query.Key{
		Path:   []string{ftQueueCtrField},
		Filter: query.NewCompValue(query.CompOpGt, arena.NewNumberFloat64(float64(threshold))),
	}

	iter, err := s.headsState.Find(filter).Iter(ctx)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var entries []HeadsStateEntry
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, err
		}

		id := doc.Value().GetString("id")
		hash := doc.Value().GetString(headsStateField)
		ctr := uint64(doc.Value().GetFloat64(ftQueueCtrField))

		entries = append(entries, HeadsStateEntry{
			ObjectID:   id,
			HeadsHash:  hash,
			FTQueueCtr: ctr,
		})
	}

	return entries, iter.Err()
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
