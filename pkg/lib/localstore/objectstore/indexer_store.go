package objectstore

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/anyenc/anyencutil"
	"github.com/anyproto/any-store/query"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var idKey = bundle.RelationKeyId.String()
var spaceIdKey = bundle.RelationKeySpaceId.String()

const ftSequenceKey = "seq"        // used to store the opstamp of the fulltext commit for specific object
const ftRecheckKey = "_ft_recheck" // used to save in

// ftQueueCounterKey returns the per-space key for FT queue consistency counter
func ftQueueCounterKey(spaceId string) string {
	return "_ft_queue_counter_" + spaceId
}

var emptyBuffer = make([]byte, 8)

// ftQueueCounter is a global atomic counter for FT queue operations
// Format: unixTimestamp * 1000 + seqNum (000-999)
var ftQueueCounter atomic.Uint64

// generateFTQueueCounter returns next monotonic counter (unixTs*1000 + seqNum)
// Uses lock-free CAS loop for thread safety
func generateFTQueueCounter() uint64 {
	for {
		current := ftQueueCounter.Load()
		currentTs := int64(current / 1000)
		currentSeq := current % 1000

		now := time.Now().Unix()
		var newVal uint64

		if now == currentTs {
			if currentSeq >= 999 {
				// Wait for next second
				time.Sleep(time.Until(time.Unix(now+1, 0)))
				continue // retry with new timestamp
			}
			newVal = current + 1
		} else {
			newVal = uint64(now) * 1000
		}

		if ftQueueCounter.CompareAndSwap(current, newVal) {
			return newVal
		}
		// CAS failed (concurrent update), retry
	}
}

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

	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	emptyVal := arena.NewBinary(emptyBuffer)
	res, err := s.fulltextQueue.Find(ftQueueFilterSeq(ftIndexSeq, query.CompOpGt, arena)).Update(txn.Context(), query.ModifyFunc(func(arena *anyenc.Arena, val *anyenc.Value) (*anyenc.Value, bool, error) {
		val.Set(ftSequenceKey, emptyVal)
		return val, true, nil
	}))

	if err != nil {
		return fmt.Errorf("create iterator: %w", err)
	}
	if res.Matched > 0 {
		log.With("seq", ftIndexSeq).Errorf("ft incosistency: found %d objects to reindex", res.Matched)
	} else {
		// no inconsistency found, we don't gc but log the count
		count, err := s.fulltextQueue.Find(ftQueueFilterSeq(0, query.CompOpGt, arena)).Count(ctx)
		if err != nil {
			return fmt.Errorf("gc fulltext queue: %w", err)
		} else {
			log.With("count", count).Infof("gc fulltext queue is valid")
		}
	}
	return txn.Commit()

}

// AddToIndexQueue adds objects to the FT queue for indexing.
// For backwards compatibility, this function does not return the counter.
func (s *dsObjectStore) AddToIndexQueue(ctx context.Context, ids ...domain.FullID) (enqueued int, err error) {
	_, enqueued, err = s.AddToIndexQueueWithCounter(ctx, ids...)
	return enqueued, err
}

// AddToIndexQueueWithCounter adds objects to the FT queue and returns a counter for consistency tracking.
// The counter is saved atomically with the queue entries to enable crash recovery.
// If the common DB doesn't flush before a crash, the counter won't be persisted,
// allowing detection of objects that were added to headsState but not to the FT queue.
func (s *dsObjectStore) AddToIndexQueueWithCounter(ctx context.Context, ids ...domain.FullID) (ftQueueCtr uint64, enqueued int, err error) {
	if len(ids) == 0 {
		return 0, 0, nil
	}

	txn, err := s.fulltextQueue.WriteTx(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("start write tx: %w", err)
	}
	arena := s.arenaPool.Get()
	defer func() {
		_ = txn.Rollback()
		arena.Reset()
		s.arenaPool.Put(arena)
	}()

	// Generate counter for this batch
	ftQueueCtr = generateFTQueueCounter()

	obj := arena.NewObject()
	var modified int
	for _, id := range ids {
		obj.Set(idKey, arena.NewString(id.ObjectID))
		obj.Set(spaceIdKey, arena.NewString(id.SpaceID))
		obj.Set(ftSequenceKey, arena.NewBinary(emptyBuffer))
		res, err := s.fulltextQueue.UpsertId(txn.Context(), id.ObjectID, query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (*anyenc.Value, bool, error) {
			if anyencutil.Equal(v, obj) {
				return v, false, nil
			}

			return obj, true, nil
		}))
		modified += res.Modified

		if err != nil {
			return 0, 0, errors.Join(txn.Rollback(), fmt.Errorf("upsert: %w", err))
		}
	}

	// Save the counter atomically in the same transaction (per-space)
	// This ensures the counter is only persisted if the queue entries are also persisted
	// All objects in batch should be from the same space
	spaceId := ids[0].SpaceID
	counterObj := arena.NewObject()
	counterObj.Set("id", arena.NewString(ftQueueCounterKey(spaceId)))
	counterObj.Set("counter", arena.NewNumberFloat64(float64(ftQueueCtr)))
	err = s.indexerChecksums.UpsertOne(txn.Context(), counterObj)
	if err != nil {
		return 0, 0, errors.Join(txn.Rollback(), fmt.Errorf("save ft queue counter: %w", err))
	}

	err = txn.Commit()
	if err != nil {
		return 0, 0, fmt.Errorf("commit: %w", err)
	}
	return ftQueueCtr, modified, nil
}

func (s *dsObjectStore) BatchProcessFullTextQueue(
	_ context.Context,
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

	arena := s.arenaPool.Get()
	defer func() {
		arena.Reset()
		s.arenaPool.Put(arena)
	}()
	filters := query.And{}
	filters = append(filters, ftQueueFilterSpaceIds(spaceIds))
	filters = append(filters, ftQueueFilterSeq(0, query.CompOpLte, arena))
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
func ftQueueFilterSeq(seq uint64, comp query.CompOp, arena *anyenc.Arena) query.Filter {
	return query.Key{
		Path:   []string{ftSequenceKey},
		Filter: query.NewCompValue(comp, ftSeq(seq, arena)),
	}
}

// ftSeq return anyenc binary value which is lexigraphically comparable
func ftSeq(seq uint64, arena *anyenc.Arena) *anyenc.Value {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, seq)
	return arena.NewBinary(buf)
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

	_, err = s.indexerChecksums.UpsertId(s.componentCtx, spaceId, query.ModifyFunc(func(a *anyenc.Arena, v *anyenc.Value) (result *anyenc.Value, modified bool, err error) {
		newVal, err := keyValueItem(a, spaceId, checksums)
		if err != nil {
			return nil, false, err
		}

		if anyencutil.Equal(newVal, v) {
			return v, false, nil
		}
		return newVal, true, nil
	}))
	return err
}

func (s *dsObjectStore) GetFTRecheckCounter(ctx context.Context) (int32, error) {
	doc, err := s.indexerChecksums.FindId(ctx, ftRecheckKey)
	if errors.Is(err, anystore.ErrDocNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return int32(doc.Value().GetInt("counter")), nil
}

func (s *dsObjectStore) SetFTRecheckCounter(ctx context.Context, counter int32) error {
	arena := s.arenaPool.Get()
	defer func() {
		s.arenaPool.Put(arena)
	}()
	obj := arena.NewObject()
	obj.Set("id", arena.NewString(ftRecheckKey))
	obj.Set("counter", arena.NewNumberInt(int(counter)))
	return s.indexerChecksums.UpsertOne(ctx, obj)
}

// GetFTQueueCounter returns the last persisted FT queue counter for a specific space.
// Used during startup to detect objects that may have been added to headsState
// but not to the FT queue due to a crash before common DB flush.
func (s *dsObjectStore) GetFTQueueCounter(ctx context.Context, spaceId string) (uint64, error) {
	doc, err := s.indexerChecksums.FindId(ctx, ftQueueCounterKey(spaceId))
	if errors.Is(err, anystore.ErrDocNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return uint64(doc.Value().GetFloat64("counter")), nil
}
