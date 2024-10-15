package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *dsObjectStore) AddToIndexQueue(ctx context.Context, ids ...domain.FullID) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	txn, err := s.fulltextQueue.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	rollback := func(err error) error {
		return errors.Join(txn.Rollback(), err)
	}

	obj := arena.NewObject()
	for _, id := range ids {
		obj.Set("id", arena.NewString(id.ObjectID))
		obj.Set("spaceId", arena.NewString(id.SpaceID))
		_, err = s.fulltextQueue.UpsertOne(txn.Context(), obj)
		if err != nil {
			return rollback(fmt.Errorf("upsert: %w", err))
		}
	}

	return txn.Commit()
}

func (s *dsObjectStore) BatchProcessFullTextQueue(ctx context.Context, spaceIdsPriority []string, limit int, processIds func(ids []string) error) error {
	proceed := 0
	for _, spaceId := range spaceIdsPriority {
		for {
			if limit <= 0 {
				return nil
			}
			ids, err := s.ListIdsFromFullTextQueue(spaceId, limit)
			if err != nil {
				return fmt.Errorf("list ids from fulltext queue: %w", err)
			}
			if len(ids) == 0 {
				break
			}

			err = processIds(ids)
			if err != nil {
				return fmt.Errorf("process ids: %w", err)
			}
			proceed += len(ids)
			err = s.RemoveIdsFromFullTextQueue(ids)
			if err != nil {
				return fmt.Errorf("remove ids from fulltext queue: %w", err)
			}
			if len(ids) < limit {
				log.Infof("fulltext queue for space %s is fully proceed; less than limit(%d)", spaceId, len(ids))
				break
			}
			limit -= len(ids)
		}
	}
	return nil
}

func (s *dsObjectStore) ListIdsFromFullTextQueue(spaceId string, limit int) ([]string, error) {
	var filter any
	if spaceId != "" {
		filter = query.Key{Path: []string{"spaceId"}, Filter: query.NewComp(query.CompOpEq, spaceId)}
	}
	if limit > math.MaxUint32 {
		limit = math.MaxUint32
	}
	iter, err := s.fulltextQueue.Find(filter).Limit(uint(limit)).Iter(s.componentCtx)
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
