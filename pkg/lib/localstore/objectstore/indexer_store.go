package objectstore

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var idKey = bundle.RelationKeyId.String()
var spaceIdKey = bundle.RelationKeySpaceId.String()

const ftSequenceKey = "seq" // used to store the opstamp of the fulltext commit for specific object

var emptyBuffer = make([]byte, 8)

// FtQueueReconcileWithSeq used to check and reindex objects on ft start in case we have consistency issues, otherwise gc the queue
// must be called before any other fulltext operations
func (s *dsObjectStore) FtQueueReconcileWithSeq(ctx context.Context, ftIndexSeq uint64) error {
	txn, err := s.fulltextQueue.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	defer func() {
		_ = txn.Rollback()
	}()

	res, err := s.fulltextQueue.Find(ftQueueFilterSeq(ftIndexSeq, query.CompOpGt)).Update(txn.Context(), query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		val.Set(ftSequenceKey, arena.NewBinary(emptyBuffer))
		return val, true, nil
	}))
	if err != nil {
		return fmt.Errorf("create iterator: %w", err)
	}
	if res.Matched > 0 {
		log.With("seq", ftIndexSeq).Errorf("ft incosistency: found %d objects to reindex", res.Matched)
	} else {
		// no inconsistency found, we can safely delete all objects with state > 0
		res, err := s.fulltextQueue.Find(ftQueueFilterSeq(0, query.CompOpGt)).Delete(txn.Context())
		if err != nil {
			return fmt.Errorf("gc fulltext queue: %w", err)
		} else if res.Matched > 0 {
			log.With("seq", ftIndexSeq).Warnf("ft queue gc: found %d objects to remove from the queue", res.Matched)
		}
	}
	return txn.Commit()

}

func (s *dsObjectStore) AddToIndexQueue(ctx context.Context, ids ...domain.FullID) error {
	txn, err := s.fulltextQueue.WriteTx(ctx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	arena := s.arenaPool.Get()
	defer func() {
		_ = txn.Rollback()
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	obj := arena.NewObject()
	for _, id := range ids {
		obj.Set(idKey, arena.NewString(id.ObjectID))
		obj.Set(spaceIdKey, arena.NewString(id.SpaceID))
		obj.Set(ftSequenceKey, arena.NewBinary(emptyBuffer))
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
) (succeedIds []domain.FullID, ftIndexSeq uint64, err error)) error {
	for {
		ids, err := s.ListIdsFromFullTextQueue(spaceIds(), limit)
		if err != nil {
			return fmt.Errorf("list ids from fulltext queue: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		succeedIds, ftIndexSeq, err := processIds(ids)
		if err != nil {
			// if all failed it will return an error and we will exit here
			return fmt.Errorf("process ids: %w", err)
		}
		if len(succeedIds) == 0 {
			// special case to prevent infinite loop
			return fmt.Errorf("all ids failed to process")
		}
		err = s.FtQueueMarkAsIndexed(succeedIds, ftIndexSeq)
		if err != nil {
			return fmt.Errorf("remove ids from fulltext queue: %w", err)
		}
	}
}

func (s *dsObjectStore) ListIdsFromFullTextQueue(spaceIds []string, limit uint) ([]domain.FullID, error) {
	if len(spaceIds) == 0 {
		return nil, fmt.Errorf("at least one space must be provided")
	}

	filters := query.And{}
	filters = append(filters, ftQueueFilterSpaceIds(spaceIds))
	filters = append(filters, ftQueueFilterSeq(0, query.CompOpLte))
	iter, err := s.fulltextQueue.Find(filters).Limit(limit).Iter(s.componentCtx)
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

func ftQueueFilterSpaceIds(spaceIds []string) query.Filter {
	if len(spaceIds) == 0 {
		return query.And{} // no filter, return all
	}
	arena := &anyenc.Arena{}
	inVals := make([]*anyenc.Value, 0, len(spaceIds))
	for _, v := range spaceIds {
		inVals = append(inVals, arena.NewString(v))
	}
	filter := query.NewInValue(inVals...)
	return query.Key{
		Path:   []string{spaceIdKey},
		Filter: filter,
	}
}

// ftQueueFilterSeq creates a filter for the fulltext queue based on sequence number
func ftQueueFilterSeq(seq uint64, comp query.CompOp) query.Filter {
	arena := &anyenc.Arena{}
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, seq)

	return query.Key{
		Path:   []string{ftSequenceKey},
		Filter: query.NewCompValue(comp, arena.NewBinary(buf)),
	}
}

func (s *dsObjectStore) FtQueueMarkAsIndexed(ids []domain.FullID, ftIndexSeq uint64) error {
	txn, err := s.fulltextQueue.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	defer func() {
		_ = txn.Rollback()
	}()

	arena := s.arenaPool.Get()
	defer func() {
		_ = txn.Rollback()
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	obj := arena.NewObject()
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, ftIndexSeq)
	obj.Set(ftSequenceKey, arena.NewBinary(buf))
	for _, id := range ids {
		obj.Set(idKey, arena.NewString(id.ObjectID))
		obj.Set(spaceIdKey, arena.NewString(id.SpaceID))
		// stateKey is set outside the loop
		err := s.fulltextQueue.UpdateOne(txn.Context(), obj)
		if errors.Is(err, anystore.ErrDocNotFound) {
			// should not happen
			log.Warnf("tried to remove %s from fulltext queue, but it was not found", id)
			continue
		}
		if err != nil {
			// if we have the error here we have nothing to do but retry later
			log.Errorf("failed to remove %s from index, will redo the fulltext index: %v", id, err)
		}
	}

	err = txn.Commit()
	if err != nil {
		return fmt.Errorf("commit write tx: %w", err)
	}

	return nil
}

func (s *dsObjectStore) ClearFullTextQueue(spaceIds []string) error {
	txn, err := s.fulltextQueue.WriteTx(s.componentCtx)
	if err != nil {
		return fmt.Errorf("start write tx: %w", err)
	}
	var commited bool
	defer func() {
		if !commited {
			txn.Rollback()
		}
	}()
	iter, err := s.fulltextQueue.Find(ftQueueFilterSpaceIds(spaceIds)).Iter(txn.Context())
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
	commited = true
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
