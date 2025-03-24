package objectstore

import (
	"context"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

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

	t.Run("remove from queue", func(t *testing.T) {
		s.RemoveIdsFromFullTextQueue([]string{"one"})
		ids, err := s.ListIdsFromFullTextQueue([]string{"id1"}, 0)
		require.NoError(t, err)

		assert.ElementsMatch(t, []domain.FullID{{ObjectID: "two", SpaceID: "id1"}}, ids)
	})
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
			func(ids []domain.FullID) ([]string, error) {
				batches = append(batches, ids)
				return lo.Map(ids, func(item domain.FullID, _ int) string { return item.ObjectID }), nil
			})
		require.NoError(t, err)
		require.Len(t, batches, 2)

		assert.ElementsMatch(t, []domain.FullID{{ObjectID: "one", SpaceID: "id1"}, {ObjectID: "two", SpaceID: "id1"}}, batches[0])
		assert.ElementsMatch(t, []domain.FullID{{ObjectID: "three", SpaceID: "id1"}}, batches[1])
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
