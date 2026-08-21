package indexer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
)

// Pins the fix for: filterOutNotChangedDocuments never filtered anything. Unchanged
// docs found in the index are skipped from changedDocs during Iterate, but the
// final loop re-adds every new doc that is not already in changedDocs —
// including those unchanged ones. As a result every object update re-indexes
// (delete+add) ALL of its docs, and the Iterate search is pure overhead.
func TestFilterOutNotChangedDocuments(t *testing.T) {
	newDocs := func() []ftsearch.SearchDoc {
		return []ftsearch.SearchDoc{
			{Id: "obj1/r/name", SpaceId: "space1", Title: "my title"},
			{Id: "obj1/b/block1", SpaceId: "space1", Text: "block text"},
		}
	}

	setup := func(t *testing.T) *fixture {
		fx := newFixture(t)
		batcher := fx.objectStore.FullText.NewAutoBatcher()
		for _, d := range newDocs() {
			require.NoError(t, batcher.UpsertDoc(d))
		}
		_, err := batcher.Finish()
		require.NoError(t, err)
		return fx
	}

	t.Run("unchanged docs are filtered out", func(t *testing.T) {
		// given
		fx := setup(t)

		// when
		changed, removed, err := fx.filterOutNotChangedDocuments("obj1", newDocs())

		// then
		require.NoError(t, err)
		assert.Empty(t, removed)
		assert.Empty(t, changed, "no doc changed, nothing should be re-indexed")
	})

	t.Run("only changed and new docs are returned", func(t *testing.T) {
		// given
		fx := setup(t)
		updated := []ftsearch.SearchDoc{
			{Id: "obj1/r/name", SpaceId: "space1", Title: "my title"},   // unchanged
			{Id: "obj1/b/block1", SpaceId: "space1", Text: "edited"},    // changed
			{Id: "obj1/b/block2", SpaceId: "space1", Text: "new block"}, // new
		}
		want := []string{"obj1/b/block1", "obj1/b/block2"}

		// when
		changed, removed, err := fx.filterOutNotChangedDocuments("obj1", updated)

		// then
		require.NoError(t, err)
		assert.Empty(t, removed)
		got := make([]string, 0, len(changed))
		for _, d := range changed {
			got = append(got, d.Id)
		}
		assert.ElementsMatch(t, want, got)
	})

	t.Run("removed docs are detected", func(t *testing.T) {
		// given
		fx := setup(t)
		updated := []ftsearch.SearchDoc{
			{Id: "obj1/r/name", SpaceId: "space1", Title: "my title"}, // unchanged; block1 is gone
		}

		// when
		changed, removed, err := fx.filterOutNotChangedDocuments("obj1", updated)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"obj1/b/block1"}, removed)
		assert.Empty(t, changed)
	})
}
