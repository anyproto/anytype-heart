package indexer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
)

func TestRemoveFullTextIndexes(t *testing.T) {
	// given
	fx := newFixture(t)
	ft := fx.objectStore.FullText
	batcher := ft.NewAutoBatcher()
	require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
		Id: "obj1/r/name", SpaceId: "space1", Title: "one",
	}))
	require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
		Id: "obj1/b/block1", SpaceId: "space1", Text: "two",
	}))
	require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
		Id: "obj2/r/name", SpaceId: "space2", Title: "three",
	}))
	_, err := batcher.Finish()
	require.NoError(t, err)

	// when
	err = fx.removeFullTextIndexes("space1")

	// then
	require.NoError(t, err)
	ids, err := ft.ListIdsBySpace("space1", 0)
	require.NoError(t, err)
	assert.Empty(t, ids, "all docs of the removed space must be deleted")

	ids, err = ft.ListIdsBySpace("space2", 0)
	require.NoError(t, err)
	assert.Len(t, ids, 1, "docs of other spaces must be kept")
}
