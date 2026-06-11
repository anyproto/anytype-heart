package ftsearch

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Repro for: BatchDeleteObjects is called with bare object ids (e.g. by
// indexer.RemoveAclIndexes), but tantivy deletes by exact term on IdRaw, which
// stores full doc paths ("objectId/r/relationKey"). A bare object id term
// matches nothing, so the object's docs survive the delete.
func TestBatchDeleteObjectsDeletesAllDocsOfObject(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "")
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	defer func() { _ = ft.Close(nil) }()

	require.NoError(t, ft.Index(SearchDoc{
		Id:      "obj1/r/name",
		SpaceId: "space1",
		Title:   "apple",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "obj1/b/block1",
		SpaceId: "space1",
		Text:    "apple pie recipe",
	}))
	require.NoError(t, ft.Index(SearchDoc{
		Id:      "obj2/r/name",
		SpaceId: "space1",
		Title:   "banana",
	}))

	require.NoError(t, ft.BatchDeleteObjects([]string{"obj1"}))

	count, err := ft.DocCount()
	require.NoError(t, err)
	assert.Equal(t, 1, int(count), "all docs of obj1 should be deleted, only obj2's doc should remain")

	results, err := ft.Search("space1", "apple", 0)
	require.NoError(t, err)
	assert.Empty(t, results, "deleted object must not be searchable")
}

// BatchDeleteObjects must also handle objects with more than one listing page
// of docs (listDocIdsForObject returns at most docLimit ids per call).
func TestBatchDeleteObjectsManyDocs(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "")
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	defer func() { _ = ft.Close(nil) }()

	const docsCount = 10_050 // more than one listing page
	batcher := ft.NewAutoBatcher()
	for i := 0; i < docsCount; i++ {
		require.NoError(t, batcher.UpsertDoc(SearchDoc{
			Id:      fmt.Sprintf("obj1/m/msg%d", i),
			SpaceId: "space1",
			Text:    "hello",
		}))
	}
	require.NoError(t, batcher.UpsertDoc(SearchDoc{
		Id:      "obj2/r/name",
		SpaceId: "space1",
		Title:   "keep me",
	}))
	_, err := batcher.Finish()
	require.NoError(t, err)

	require.NoError(t, ft.BatchDeleteObjects([]string{"obj1"}))

	count, err := ft.DocCount()
	require.NoError(t, err)
	assert.Equal(t, 1, int(count))
}

func TestListIdsBySpace(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "")
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	defer func() { _ = ft.Close(nil) }()

	require.NoError(t, ft.Index(SearchDoc{Id: "obj1/r/name", SpaceId: "space1", Title: "one"}))
	require.NoError(t, ft.Index(SearchDoc{Id: "obj1/b/block1", SpaceId: "space1", Text: "two"}))
	require.NoError(t, ft.Index(SearchDoc{Id: "obj2/r/name", SpaceId: "space2", Title: "three"}))

	ids, err := ft.ListIdsBySpace("space1", 0)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"obj1/r/name", "obj1/b/block1"}, ids)

	ids, err = ft.ListIdsBySpace("space2", 0)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"obj2/r/name"}, ids)

	ids, err = ft.ListIdsBySpace("spaceUnknown", 0)
	require.NoError(t, err)
	assert.Empty(t, ids)

	// limit is respected
	ids, err = ft.ListIdsBySpace("space1", 1)
	require.NoError(t, err)
	assert.Len(t, ids, 1)
}

// Repro for: Search caps results at 100 docs (SetDocsLimit(100)) before any
// objectstore-level filtering happens downstream. Matches beyond the cap are
// silently dropped, which breaks recall for filtered searches, pagination
// beyond the cap, and chat search (messages compete with all other docs for
// the same 100 slots).
func TestSearchReturnsAllMatches(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "")
	fixture := newFixture(tmpDir, t)
	ft := fixture.ft
	defer func() { _ = ft.Close(nil) }()

	const docsCount = 150
	batcher := ft.NewAutoBatcher()
	for i := 0; i < docsCount; i++ {
		require.NoError(t, batcher.UpsertDoc(SearchDoc{
			Id:      fmt.Sprintf("obj%d/r/name", i),
			SpaceId: "space1",
			Title:   "apple",
		}))
	}
	_, err := batcher.Finish()
	require.NoError(t, err)

	results, err := ft.Search("space1", "apple", docsCount)
	require.NoError(t, err)
	assert.Len(t, results, docsCount, "all matching docs should be returned (or the limit must be caller-controlled)")
}
