package objectstore

import (
	"context"
	"fmt"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-store/syncpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func listIdsFromFullTextQueueAll(ftqueue anystore.Collection, spaceIds []string, limit uint) ([]domain.FullID, error) {
	if len(spaceIds) == 0 {
		return nil, fmt.Errorf("at least one space must be provided")
	}

	filters := query.And{}
	filters = append(filters, ftQueueFilterSpaceIds(spaceIds))
	// filters = append(filters, ftQueueFilterSeq(0, query.CompOpLte))
	iter, err := ftqueue.Find(filters).Limit(limit).Iter(context.Background())
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

func TestDsObjectStore_IndexQueue(t *testing.T) {
	s := NewStoreFixture(t)

	ctx := context.Background()

	t.Run("add to queue", func(t *testing.T) {
		require.NoError(t, s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "one", SpaceID: "id1"}))
		require.NoError(t, s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "one", SpaceID: "id1"}))
		require.NoError(t, s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "two", SpaceID: "id1"}))

		ids, err := s.ListIdsFromFullTextQueue([]string{"id1"}, 0)
		require.NoError(t, err)

		assert.ElementsMatch(t, []domain.FullID{{ObjectID: "one", SpaceID: "id1"}, {ObjectID: "two", SpaceID: "id1"}}, ids)
	})

	t.Run("reconcile", func(t *testing.T) {
		require.NoError(t, s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "x", SpaceID: "id2"}))
		require.NoError(t, s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "y", SpaceID: "id2"}))
		require.NoError(t, s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "z", SpaceID: "id2"}))
		ids, err := s.ListIdsFromFullTextQueue([]string{"id2"}, 0)
		require.NoError(t, err)
		require.Len(t, ids, 3)

		require.NoError(t, s.FtQueueMarkAsIndexed([]domain.FullID{{ObjectID: "x", SpaceID: "id2"}}, 1))
		require.NoError(t, s.FtQueueMarkAsIndexed([]domain.FullID{{ObjectID: "y", SpaceID: "id2"}}, 2))
		require.NoError(t, s.FtQueueMarkAsIndexed([]domain.FullID{{ObjectID: "z", SpaceID: "id2"}}, 3))

		ids, err = s.ListIdsFromFullTextQueue([]string{"id2"}, 0)
		require.NoError(t, err)
		require.Len(t, ids, 0)

		err = s.FtQueueReconcileWithSeq(context.Background(), 1)
		require.NoError(t, err)

		ids, err = s.ListIdsFromFullTextQueue([]string{"id2"}, 0)
		require.NoError(t, err)
		require.Len(t, ids, 2)
	})
}

func Test_ftSeq(t *testing.T) {
	arena := &anyenc.Arena{}

	seq0 := ftSeq(uint64(0), arena)
	seq1 := ftSeq(uint64(1), arena)
	seq2 := ftSeq(uint64(2), arena)

	val := arena.NewObject()

	docBuf := &syncpool.DocBuffer{}
	filterGt1 := ftQueueFilterSeq(1, query.CompOpGt, arena)
	val.Set(ftSequenceKey, seq0)
	assert.False(t, filterGt1.Ok(val, docBuf))

	val.Set(ftSequenceKey, seq1)
	assert.False(t, filterGt1.Ok(val, docBuf))

	val.Set(ftSequenceKey, seq2)
	assert.True(t, filterGt1.Ok(val, docBuf))

	filterGt0 := ftQueueFilterSeq(0, query.CompOpGt, arena)
	assert.True(t, filterGt0.Ok(val, docBuf))

	emptyBufferVal := arena.NewBinary(emptyBuffer)
	val.Set(ftSequenceKey, emptyBufferVal)
	filterLte0 := ftQueueFilterSeq(0, query.CompOpLte, arena)
	assert.True(t, filterLte0.Ok(val, docBuf))
}

func TestIndexerBatch(t *testing.T) {
	s := NewStoreFixture(t)
	ctx := context.Background()

	t.Run("batch - no more than limit", func(t *testing.T) {
		require.NoError(t, s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "one", SpaceID: "id1"}))
		require.NoError(t, s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "two", SpaceID: "id1"}))
		require.NoError(t, s.AddToIndexQueue(ctx, domain.FullID{ObjectID: "three", SpaceID: "id1"}))
		var batches [][]domain.FullID
		err := s.BatchProcessFullTextQueue(
			context.Background(),
			func() []string { return []string{"id1"} },
			2,
			func(ids []domain.FullID) ([]domain.FullID, uint64, error) {
				batches = append(batches, ids)
				return ids, 1, nil
			})
		require.NoError(t, err)
		require.Len(t, batches, 2)

		// Collect all processed IDs
		var allProcessed []domain.FullID
		for _, batch := range batches {
			allProcessed = append(allProcessed, batch...)
		}

		// Verify all items were processed
		assert.ElementsMatch(t, []domain.FullID{
			{ObjectID: "one", SpaceID: "id1"},
			{ObjectID: "two", SpaceID: "id1"},
			{ObjectID: "three", SpaceID: "id1"},
		}, allProcessed)

		// Verify batch sizes
		assert.LessOrEqual(t, len(batches[0]), 2)
		assert.LessOrEqual(t, len(batches[1]), 2)
	})
}

func TestIndexerChecksums(t *testing.T) {
	t.Run("save and load checksums", func(t *testing.T) {
		s := NewStoreFixture(t)

		want := &model.ObjectStoreChecksums{
			BundledObjectTypes:               "hash1",
			BundledRelations:                 "hash2",
			BundledLayouts:                   "hash3",
			ObjectsForceReindexCounter:       1,
			FilesForceReindexCounter:         2,
			IdxRebuildCounter:                3,
			BundledTemplates:                 "hash4",
			BundledObjects:                   5,
			FilestoreKeysForceReindexCounter: 6,
		}

		require.NoError(t, s.SaveChecksums("spaceX", want))

		got, err := s.GetChecksums("spaceX")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}
