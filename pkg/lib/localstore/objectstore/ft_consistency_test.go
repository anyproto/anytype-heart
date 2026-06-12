package objectstore

import (
	"context"
	"fmt"
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
		checked, enqueued, complete, err := s.RunFTConsistencyCheck(ctx, s.FullText)

		// then
		require.NoError(t, err)
		assert.Equal(t, 2, checked)
		assert.Equal(t, 1, enqueued)
		assert.True(t, complete)

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

	t.Run("soft-deleted store stubs do not shield their ft docs", func(t *testing.T) {
		// given: a soft-deleted object keeps a store stub (isDeleted=true) but
		// its leftover FT docs are garbage and must be collected
		s := NewStoreFixture(t)
		s.AddObjects(t, "space1", []TestObject{
			{
				bundle.RelationKeyId:      domain.String("aliveObj"),
				bundle.RelationKeySpaceId: domain.String("space1"),
				bundle.RelationKeyName:    domain.String("alive"),
			},
			{
				bundle.RelationKeyId:        domain.String("softDeletedObj"),
				bundle.RelationKeySpaceId:   domain.String("space1"),
				bundle.RelationKeyIsDeleted: domain.Bool(true),
			},
		})
		batcher := s.FullText.NewAutoBatcher()
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id: "aliveObj/r/name", SpaceId: "space1", Title: "alive",
		}))
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id: "softDeletedObj/r/name", SpaceId: "space1", Title: "leftover",
		}))
		_, err := batcher.Finish()
		require.NoError(t, err)

		// when
		_, _, complete, err := s.RunFTConsistencyCheck(ctx, s.FullText)

		// then
		require.NoError(t, err)
		assert.True(t, complete)
		leftoverDocs, err := s.FullText.ListIdsBySpace("space1", 0)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"aliveObj/r/name"}, leftoverDocs,
			"soft-deleted object's ft docs must be garbage-collected, alive object's kept")
	})

	t.Run("orphan gc is skipped when the store is empty", func(t *testing.T) {
		// given: a space whose store exists but holds no objects (e.g. wiped or
		// not yet rebuilt) while the FT index still has its docs — deleting
		// them all would be a destructive misclassification
		s := NewStoreFixture(t)
		// registers the (empty) space store
		_ = s.SpaceIndex("space1")
		batcher := s.FullText.NewAutoBatcher()
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id: "obj1/r/name", SpaceId: "space1", Title: "object",
		}))
		_, err := batcher.Finish()
		require.NoError(t, err)

		// when
		_, _, complete, err := s.RunFTConsistencyCheck(ctx, s.FullText)

		// then
		require.NoError(t, err)
		assert.False(t, complete, "a skipped space must mark the run incomplete so it is retried next session")
		docs, err := s.FullText.ListIdsBySpace("space1", 0)
		require.NoError(t, err)
		assert.Len(t, docs, 1, "ft docs of an empty store must not be collected as orphans")
	})

	t.Run("orphan gc is skipped when the store is implausibly sparse", func(t *testing.T) {
		// given: a store with a single object while the FT index holds many —
		// the signature of a wiped store mid-rebuild, not of genuine orphans
		s := NewStoreFixture(t)
		s.AddObjects(t, "space1", []TestObject{
			{
				bundle.RelationKeyId:      domain.String("survivor"),
				bundle.RelationKeySpaceId: domain.String("space1"),
				bundle.RelationKeyName:    domain.String("survivor"),
			},
		})
		batcher := s.FullText.NewAutoBatcher()
		const ftObjects = 15 // > ftOrphanStoreRatio * 1 store object
		for i := 0; i < ftObjects; i++ {
			require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
				Id:      fmt.Sprintf("obj%d/r/name", i),
				SpaceId: "space1",
				Title:   "object",
			}))
		}
		_, err := batcher.Finish()
		require.NoError(t, err)

		// when
		_, _, complete, err := s.RunFTConsistencyCheck(ctx, s.FullText)

		// then
		require.NoError(t, err)
		assert.False(t, complete)
		docs, err := s.FullText.ListIdsBySpace("space1", 0)
		require.NoError(t, err)
		assert.Len(t, docs, ftObjects, "docs of an implausibly sparse store must not be collected")
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
		_, _, _, err = s.RunFTConsistencyCheck(ctx, s.FullText)

		// then
		require.NoError(t, err)
		foreignDocs, err := s.FullText.ListByIdPrefix("foreignObj/")
		require.NoError(t, err)
		assert.Len(t, foreignDocs, 1, "docs of spaces that were not iterated must not be deleted")
	})
}
