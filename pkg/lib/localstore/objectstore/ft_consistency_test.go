package objectstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
)

func TestRunFTConsistencyCheck(t *testing.T) {
	ctx := context.Background()

	t.Run("missing objects are enqueued, orphaned docs are deleted", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)
		// objInStoreAndFt: consistent — in store and in FT
		// objMissing: in store, not in FT — must be enqueued
		// objOrphan: in FT only — its docs must be deleted
		s.AddObjects(t, "space1", []TestObject{
			{
				bundle.RelationKeyId:      domain.String("objInStoreAndFt"),
				bundle.RelationKeySpaceId: domain.String("space1"),
				bundle.RelationKeyName:    domain.String("consistent object"),
			},
			{
				bundle.RelationKeyId:      domain.String("objMissing"),
				bundle.RelationKeySpaceId: domain.String("space1"),
				bundle.RelationKeyName:    domain.String("missing object"),
			},
		})

		batcher := s.FullText.NewAutoBatcher()
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id: "objInStoreAndFt/r/name", SpaceId: "space1", Title: "consistent object",
		}))
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id: "objOrphan/r/name", SpaceId: "space1", Title: "orphan",
		}))
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id: "objOrphan/b/block1", SpaceId: "space1", Text: "orphan block",
		}))
		_, err := batcher.Finish()
		require.NoError(t, err)

		// when
		checked, enqueued, err := s.RunFTConsistencyCheck(ctx, s.FullText)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, checked)
		assert.Equal(t, 1, enqueued)

		queued, err := s.ListIdsFromFullTextQueue([]string{"space1"}, 0)
		require.NoError(t, err)
		require.Len(t, queued, 1)
		assert.Equal(t, "objMissing", queued[0].ObjectId)

		orphanDocs, err := s.FullText.ListByIdPrefix("objOrphan/")
		require.NoError(t, err)
		assert.Empty(t, orphanDocs, "orphaned ft docs must be garbage-collected")

		keptDocs, err := s.FullText.ListByIdPrefix("objInStoreAndFt/")
		require.NoError(t, err)
		assert.Len(t, keptDocs, 1, "consistent object's docs must be kept")
	})

	t.Run("docs of not-iterated spaces are never touched", func(t *testing.T) {
		// given: docs in FT for a space that has no store (not loaded/iterated)
		s := NewStoreFixture(t)
		s.AddObjects(t, "space1", []TestObject{
			{
				bundle.RelationKeyId:      domain.String("obj1"),
				bundle.RelationKeySpaceId: domain.String("space1"),
				bundle.RelationKeyName:    domain.String("object"),
			},
		})
		batcher := s.FullText.NewAutoBatcher()
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id: "obj1/r/name", SpaceId: "space1", Title: "object",
		}))
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id: "foreignObj/r/name", SpaceId: "spaceNotLoaded", Title: "foreign",
		}))
		_, err := batcher.Finish()
		require.NoError(t, err)

		// when
		_, _, err = s.RunFTConsistencyCheck(ctx, s.FullText)

		// then
		require.NoError(t, err)
		foreignDocs, err := s.FullText.ListByIdPrefix("foreignObj/")
		require.NoError(t, err)
		assert.Len(t, foreignDocs, 1, "docs of spaces that were not iterated must not be deleted")
	})
}
