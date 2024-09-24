package objectstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestDsObjectStore_IndexQueue(t *testing.T) {
	s := NewStoreFixture(t)

	t.Run("add to queue", func(t *testing.T) {
		require.NoError(t, s.AddToIndexQueue(domain.FullID{ObjectID: "one", SpaceID: "space1"}, domain.FullID{ObjectID: "two", SpaceID: "space1"}))
		require.NoError(t, s.AddToIndexQueue(domain.FullID{ObjectID: "one", SpaceID: "space1"}))

		ids, err := s.ListIDsFromFullTextQueue("space1", 0)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"one", "two"}, ids)
	})

	t.Run("remove from queue", func(t *testing.T) {
		s.RemoveIDsFromFullTextQueue([]string{"one"})
		ids, err := s.ListIDsFromFullTextQueue("space1", 0)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"two"}, ids)
	})
}

func TestIndexerBatch(t *testing.T) {
	s := NewStoreFixture(t)

	t.Run("batch - no more than limit", func(t *testing.T) {
		require.NoError(t, s.AddToIndexQueue(domain.FullID{ObjectID: "one", SpaceID: "space1"}, domain.FullID{ObjectID: "two", SpaceID: "space1"}))
		require.NoError(t, s.AddToIndexQueue(domain.FullID{ObjectID: "three", SpaceID: "space2"}))

		var batches [][]string
		err := s.BatchProcessFullTextQueue(context.Background(), []string{"space2", "space1"}, 2, func(ids []string) error {
			batches = append(batches, ids)
			return nil
		})
		require.NoError(t, err)
		require.Len(t, batches, 2)

		assert.ElementsMatch(t, []string{"three"}, batches[0]) // priority for space2
		assert.ElementsMatch(t, []string{"one", "two"}, batches[1])

	})
}

func TestIndexerChecksums(t *testing.T) {
	t.Run("previous checksums are not found", func(t *testing.T) {
		s := NewStoreFixture(t)

		_, err := s.GetGlobalChecksums()
		require.Error(t, err)
	})

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

func TestHeadsHash(t *testing.T) {
	t.Run("previous hash is not found", func(t *testing.T) {
		s := NewStoreFixture(t)

		got, err := s.GetLastIndexedHeadsHash("id1")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("save and load hash", func(t *testing.T) {
		s := NewStoreFixture(t)

		want := "hash1"

		require.NoError(t, s.SaveLastIndexedHeadsHash("id1", want))

		got, err := s.GetLastIndexedHeadsHash("id1")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}
