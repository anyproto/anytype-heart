package objectstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestDsObjectStore_IndexQueue(t *testing.T) {
	s := NewStoreFixture(t)

	t.Run("add to queue", func(t *testing.T) {
		require.NoError(t, s.AddToIndexQueue("one"))
		require.NoError(t, s.AddToIndexQueue("one"))
		require.NoError(t, s.AddToIndexQueue("two"))

		ids, err := s.ListIDsFromFullTextQueue()
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"one", "two"}, ids)
	})

	t.Run("remove from queue", func(t *testing.T) {
		s.RemoveIDsFromFullTextQueue([]string{"one"})
		ids, err := s.ListIDsFromFullTextQueue()
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"two"}, ids)
	})
}

func TestIndexerChecksums(t *testing.T) {
	t.Run("previous checksums are not found", func(t *testing.T) {
		s := NewStoreFixture(t)

		_, err := s.GetChecksums()
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
			FulltextRebuild:                  4,
			BundledTemplates:                 "hash4",
			BundledObjects:                   5,
			FilestoreKeysForceReindexCounter: 6,
		}

		require.NoError(t, s.SaveChecksums(want))

		got, err := s.GetChecksums()
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
