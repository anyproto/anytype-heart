package objectstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestDsObjectStore_IndexQueue(t *testing.T) {
	s := NewStoreFixture(t)

	t.Run("add to queue", func(t *testing.T) {
		require.NoError(t, s.AddToIndexQueue(ctx, "one"))
		require.NoError(t, s.AddToIndexQueue(ctx, "one"))
		require.NoError(t, s.AddToIndexQueue(ctx, "two"))

		ids, err := s.ListIdsFromFullTextQueue(0)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"one", "two"}, ids)
	})

	t.Run("remove from queue", func(t *testing.T) {
		s.RemoveIdsFromFullTextQueue([]string{"one"})
		ids, err := s.ListIdsFromFullTextQueue(0)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"two"}, ids)
	})
}

func TestIndexerBatch(t *testing.T) {
	s := NewStoreFixture(t)

	t.Run("batch - no more than limit", func(t *testing.T) {
		require.NoError(t, s.AddToIndexQueue(ctx, "one"))
		require.NoError(t, s.AddToIndexQueue(ctx, "two"))
		require.NoError(t, s.AddToIndexQueue(ctx, "three"))

		var batches [][]string
		err := s.BatchProcessFullTextQueue(context.Background(), 2, func(ids []string) error {
			batches = append(batches, ids)
			return nil
		})
		require.NoError(t, err)
		require.Len(t, batches, 2)

		assert.ElementsMatch(t, []string{"one", "two"}, batches[0])
		assert.ElementsMatch(t, []string{"three"}, batches[1])
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
