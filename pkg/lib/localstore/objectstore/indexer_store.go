package objectstore

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

func (s *dsObjectStore) AddToIndexQueue(id string) error {
	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()
	obj := arena.NewObject()
	obj.Set("id", arena.NewString(id))

	_, err := s.fulltextQueue.UpsertOne(s.componentCtx, obj)
	return err
}

func (s *dsObjectStore) BatchProcessFullTextQueue(ctx context.Context, limit int, processIds func(ids []string) error) error {
	ids, err := s.ListIDsFromFullTextQueue(limit)
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
	return s.RemoveIDsFromFullTextQueue(ids)
}

func (s *dsObjectStore) ListIDsFromFullTextQueue(limit int) ([]string, error) {
	iter, err := s.fulltextQueue.Find(nil).Limit(uint(limit)).Iter(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("create iterator: %w", err)
	}
	var ids []string
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, errors.Join(iter.Close(), fmt.Errorf("read doc: %w", err))
		}
		id := doc.Value().GetStringBytes("id")
		ids = append(ids, string(id))
	}
	return ids, iter.Close()
}

func (s *dsObjectStore) RemoveIDsFromFullTextQueue(ids []string) error {
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

func (s *dsObjectStore) GetChecksums(spaceID string) (checksums *model.ObjectStoreChecksums, err error) {
	return badgerhelper.GetValue(s.db, bundledChecksums.ChildString(spaceID).Bytes(), func(raw []byte) (*model.ObjectStoreChecksums, error) {
		checksums := &model.ObjectStoreChecksums{}
		return checksums, proto.Unmarshal(raw, checksums)
	})
}

func (s *dsObjectStore) SaveChecksums(spaceID string, checksums *model.ObjectStoreChecksums) (err error) {
	// in case we have global checksums we need to remove them, because it should not be used for any new space
	if spaceID != addr.AnytypeMarketplaceWorkspace {
		_ = badgerhelper.DeleteValue(s.db, bundledChecksums.Bytes())
	}
	return badgerhelper.SetValue(s.db, bundledChecksums.ChildString(spaceID).Bytes(), checksums)
}

// GetGlobalChecksums is a migration method, it returns checksums stored before we started to store them per space
// it will be deleted after the first SaveChecksums() call
func (s *dsObjectStore) GetGlobalChecksums() (checksums *model.ObjectStoreChecksums, err error) {
	return badgerhelper.GetValue(s.db, bundledChecksums.Bytes(), func(raw []byte) (*model.ObjectStoreChecksums, error) {
		checksums := &model.ObjectStoreChecksums{}
		return checksums, proto.Unmarshal(raw, checksums)
	})
}

// GetLastIndexedHeadsHash return empty hash without error if record was not found
func (s *dsObjectStore) GetLastIndexedHeadsHash(id string) (headsHash string, err error) {
	headsHash, err = badgerhelper.GetValue(s.db, indexedHeadsState.ChildString(id).Bytes(), bytesToString)
	if err != nil && !badgerhelper.IsNotFound(err) {
		return "", err
	}
	return headsHash, nil
}

func (s *dsObjectStore) SaveLastIndexedHeadsHash(id string, headsHash string) (err error) {
	return badgerhelper.SetValue(s.db, indexedHeadsState.ChildString(id).Bytes(), headsHash)
}
