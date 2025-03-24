package objectstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var idKey = bundle.RelationKeyId.String()
var spaceIdKey = bundle.RelationKeySpaceId.String()

func (s *dsObjectStore) AddToIndexQueue(ctx context.Context, ids ...domain.FullID) error {
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
		obj.Set(idKey, arena.NewString(id.ObjectID))
		obj.Set(spaceIdKey, arena.NewString(id.SpaceID))
		err = s.fulltextQueue.UpsertOne(txn.Context(), obj)
		if err != nil {
			return errors.Join(txn.Rollback(), fmt.Errorf("upsert: %w", err))
		}
	}
	return txn.Commit()
}

func (s *dsObjectStore) BatchProcessFullTextQueue(
	ctx context.Context,
	spaceIds func() []string,
	limit uint,
	processIds func(objectIds []domain.FullID,
	) ([]string, error)) error {
	for {
		ids, err := s.ListIdsFromFullTextQueue(spaceIds(), limit)
		if err != nil {
			return fmt.Errorf("list ids from fulltext queue: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		toRemove, err := processIds(ids)
		if err != nil {
			return fmt.Errorf("process ids: %w", err)
		}
		err = s.RemoveIdsFromFullTextQueue(toRemove)
		if err != nil {
			return fmt.Errorf("remove ids from fulltext queue: %w", err)
		}
	}
}

func (s *dsObjectStore) ListIdsFromFullTextQueue(spaceIds []string, limit uint) ([]domain.FullID, error) {
	if len(spaceIds) == 0 {
		return nil, fmt.Errorf("at least one space must be provided")
	}

	filterIn := inSpaceIds(spaceIds)

	iter, err := s.fulltextQueue.Find(filterIn.AnystoreFilter()).Limit(limit).Iter(s.componentCtx)
	if err != nil {
		return nil, fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	var ids []domain.FullID
	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return nil, fmt.Errorf("read doc: %w", err)
		}
		id := doc.Value().GetString(idKey)
		spaceId := doc.Value().GetString(spaceIdKey)
		ids = append(ids, domain.FullID{ObjectID: id, SpaceID: spaceId})
	}
	return ids, nil
}

func inSpaceIds(spaceIds []string) database.FilterIn {
	sourceList := make([]domain.Value, 0, len(spaceIds))
	for _, id := range spaceIds {
		sourceList = append(sourceList, domain.String(id))
	}
	filterIn := database.FilterIn{
		Key:   bundle.RelationKeySpaceId,
		Value: sourceList,
	}
	return filterIn
}

func (s *dsObjectStore) RemoveIdsFromFullTextQueue(ids []string) error {
	txn, err := s.fulltextQueue.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	for _, id := range ids {
		err := s.fulltextQueue.DeleteId(txn.Context(), id)
		if errors.Is(err, anystore.ErrDocNotFound) {
			continue
		}
		if err != nil {
			// if we have the error here we have nothing to do but retry later
			log.Errorf("failed to remove %s from index, will redo the fulltext index: %v", id, err)
		}
	}
	return txn.Commit()
}

func (s *dsObjectStore) ClearFullTextQueue(ctx context.Context, spaceIds ...string) error {
	var filterIn database.FilterIn
	if len(spaceIds) > 0 {
		filterIn = inSpaceIds(spaceIds)
	}
	txn, err := s.fulltextQueue.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	iter, err := s.fulltextQueue.Find(filterIn).Iter(s.componentCtx)
	if err != nil {
		return fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return fmt.Errorf("read doc: %w", err)
		}
		id := doc.Value().GetString(idKey)
		err = s.fulltextQueue.DeleteId(txn.Context(), id)
		if err != nil {
			return fmt.Errorf("del doc: %w", err)
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
	err = s.indexerChecksums.UpsertOne(s.componentCtx, it)
	return err
}
