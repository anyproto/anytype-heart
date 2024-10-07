package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *dsObjectStore) AddToIndexQueue(ctx context.Context, ids ...string) error {
	txn, err := s.fulltextQueue.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	obj := arena.NewObject()
	for _, id := range ids {
		obj.Set("id", arena.NewString(id))
		_, err = s.fulltextQueue.UpsertOne(txn.Context(), obj)
		if err != nil {
			return errors.Join(txn.Rollback(), fmt.Errorf("upsert: %w", err))
		}
	}
	return txn.Commit()
}

func (s *dsObjectStore) BatchProcessFullTextQueue(ctx context.Context, limit int, processIds func(ids []string) error) error {
	for {
		ids, err := s.ListIdsFromFullTextQueue(limit)
		if err != nil {
			return fmt.Errorf("list ids from fulltext queue: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		err = processIds(ids)
		if err != nil {
			return fmt.Errorf("process ids: %w", err)
		}
		err = s.RemoveIdsFromFullTextQueue(ids)
		if err != nil {
			return fmt.Errorf("remove ids from fulltext queue: %w", err)
		}
	}
}

func (s *dsObjectStore) ListIdsFromFullTextQueue(limit int) ([]string, error) {
	iter, err := s.fulltextQueue.Find(nil).Limit(uint(limit)).Iter(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	var ids []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("read doc: %w", err)
		}
		id := doc.Value().GetStringBytes("id")
		ids = append(ids, string(id))
	}
	return ids, nil
}

func (s *dsObjectStore) RemoveIdsFromFullTextQueue(ids []string) error {
	txn, err := s.fulltextQueue.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	for _, id := range ids {
		err := s.fulltextQueue.DeleteId(txn.Context(), id)
		if err != nil {
			// if we have the error here we have nothing to do but retry later
			log.Errorf("failed to remove %s from index, will redo the fulltext index: %v", id, err)
		}
	}
	return txn.Commit()
}

func (s *dsObjectStore) GetChecksums(spaceID string) (*model.ObjectStoreChecksums, error) {
	doc, err := s.indexerChecksums.FindId(s.componentCtx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("get checksums: %w", err)
	}
	val := doc.Value().GetStringBytes("value")
	var checksums *model.ObjectStoreChecksums
	err = json.Unmarshal(val, &checksums)
	return checksums, err
}

func (s *dsObjectStore) SaveChecksums(spaceId string, checksums *model.ObjectStoreChecksums) (err error) {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	it, err := keyValueItem(arena, spaceId, checksums)
	if err != nil {
		return err
	}
	_, err = s.indexerChecksums.UpsertOne(s.componentCtx, it)
	return err
}

// GetGlobalChecksums is a migration method, it returns checksums stored before we started to store them per space
// it will be deleted after the first SaveChecksums() call
func (s *dsObjectStore) GetGlobalChecksums() (checksums *model.ObjectStoreChecksums, err error) {
	// TODO What to do?
	return s.GetChecksums("global")
}
