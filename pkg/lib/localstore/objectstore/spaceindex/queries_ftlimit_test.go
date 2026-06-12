package spaceindex

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
)

func TestFtCandidatesLimit(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limit  int
		offset int
		want   int
	}{
		{"no limit gets one conservative round", 0, 0, ftCandidatesMin},
		{"small page is padded to the minimum", 10, 0, ftCandidatesMin},
		{"offset counts towards the multiplied budget", 50, 80, 260},
		{"big page is granted upfront with headroom", 400, 300, 1400},
		{"budget is capped by the hard limit", 900, 500, ftCandidatesHardLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ftCandidatesLimit(database.Query{Limit: tc.limit, Offset: tc.offset})
			assert.Equal(t, tc.want, got)
		})
	}
}

// Clients treat a page shorter than the requested limit as the end of the
// result set. Store-level filters run after the fulltext search, so the
// candidate budget must escalate until the page is filled or the index is
// genuinely exhausted — otherwise filtered-out candidates produce a short page
// while more matches exist.
func TestFulltextQueryFillsThePage(t *testing.T) {
	// given: 250 objects matching the query, 170 of them filtered out by the
	// default isArchived filter; only 80 results actually exist. The counts are
	// chosen so even the multiplied first-round budget (2x the page) cannot
	// fill the page and escalation must kick in.
	fx := NewStoreFixture(t)
	const (
		total    = 250
		archived = 170
	)
	objects := make([]TestObject, 0, total)
	batcher := fx.fts.NewAutoBatcher()
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("obj%d", i)
		obj := TestObject{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyName: domain.String("apple"),
		}
		if i < archived {
			obj[bundle.RelationKeyIsArchived] = domain.Bool(true)
		}
		objects = append(objects, obj)
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id:      id + "/r/name",
			SpaceId: "test",
			Title:   "apple",
		}))
	}
	_, err := batcher.Finish()
	require.NoError(t, err)
	fx.AddObjects(t, objects)

	t.Run("page is filled past the first candidate round", func(t *testing.T) {
		// when: the first round (2x100 = 200 docs) can contain at most 80
		// non-archived objects, so filling a 100-record page needs escalation
		records, err := fx.Query(database.Query{
			SpaceId:   "test",
			TextQuery: "apple",
			Limit:     100,
		})

		// then: every existing result is returned; a shorter page would make
		// the client stop paginating while matches still exist
		require.NoError(t, err)
		assert.Len(t, records, total-archived)
	})

	t.Run("offset pages reach results beyond the first round", func(t *testing.T) {
		// when
		records, err := fx.Query(database.Query{
			SpaceId:   "test",
			TextQuery: "apple",
			Limit:     50,
			Offset:    60,
		})

		// then: 80 results exist, the page covers 60..79
		require.NoError(t, err)
		assert.Len(t, records, 20)
	})
}

// The tail beyond the re-rank head is already in final order, so it must be
// resolved lazily: once the requested page is covered, the remaining
// candidates must not cost store reads.
func TestFulltextResolveStopsAtMaxRecords(t *testing.T) {
	// given
	fx := NewStoreFixture(t)
	const total = 30
	objects := make([]TestObject, 0, total)
	results := make([]database.FulltextResult, 0, total)
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("obj%03d", i)
		objects = append(objects, TestObject{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyName: domain.String("match"),
		})
		results = append(results, database.FulltextResult{
			Path:  domain.ObjectPath{ObjectId: id, RelationKey: "name"},
			Score: float64(total-i) / 10,
		})
	}
	fx.AddObjects(t, objects)
	params := newFilters(t, fx, nil, nil)

	// when
	records, _ := fx.resolveFulltextResults(results, params, "match", map[string]struct{}{}, false, 10)

	// then: resolution stops at the cap, in input order
	require.Len(t, records, 10)
	assert.Equal(t, "obj000", records[0].Details.GetString(bundle.RelationKeyId))
	assert.Equal(t, "obj009", records[9].Details.GetString(bundle.RelationKeyId))
}

// Offset pagination is only sound when the result order does not depend on
// the candidate budget (which grows with the requested offset). The
// final-score boosts may therefore re-rank only a fixed head of the BM25
// order: re-ranking the whole budget-dependent pool would let a
// recency-boosted tail candidate jump into an earlier page, shifting
// everything below it and producing duplicates on the next page.
func TestFulltextPaginationConsistency(t *testing.T) {
	// given: 150 matching objects with descending BM25 scores (longer titles
	// score lower); one deep-tail object is freshly modified, so a global
	// re-rank would promote it across pages
	fx := NewStoreFixture(t)
	const (
		total    = 150
		pageSize = 20
		recentId = "obj140"
	)
	objects := make([]TestObject, 0, total)
	batcher := fx.fts.NewAutoBatcher()
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("obj%03d", i)
		title := "apple" + strings.Repeat(" filler", i/5)
		obj := TestObject{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyName: domain.String(title),
		}
		if id == recentId {
			obj[bundle.RelationKeyLastModifiedDate] = domain.Int64(time.Now().Unix())
		}
		objects = append(objects, obj)
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id:      id + "/r/name",
			SpaceId: "test",
			Title:   title,
		}))
	}
	_, err := batcher.Finish()
	require.NoError(t, err)
	fx.AddObjects(t, objects)

	query := func(limit, offset int) []string {
		records, err := fx.Query(database.Query{
			SpaceId:   "test",
			TextQuery: "apple",
			Limit:     limit,
			Offset:    offset,
		})
		require.NoError(t, err)
		ids := make([]string, 0, len(records))
		for _, r := range records {
			ids = append(ids, r.Details.GetString(bundle.RelationKeyId))
		}
		return ids
	}

	// when: one big page vs. many small pages (each request computes a
	// different candidate budget)
	full := query(total, 0)
	require.Len(t, full, total)

	var paged []string
	for offset := 0; ; offset += pageSize {
		page := query(pageSize, offset)
		if len(page) == 0 {
			break
		}
		paged = append(paged, page...)
	}

	// then: pagination yields exactly the same sequence — no duplicates, no
	// gaps, no order drift between requests
	assert.Equal(t, full, paged)

	// and the boosted deep-tail object stays in the BM25 tail instead of
	// jumping into the first page past candidates of earlier requests
	assert.NotContains(t, full[:pageSize], recentId)
	assert.Contains(t, full, recentId)
}

// The re-rank head must be the same object set for every request. When objects
// have multiple matching docs, a doc-count budget yields fewer objects than
// docs, so a small request could otherwise re-rank a smaller head than a
// larger request and the two would paginate different sequences.
func TestFulltextPaginationConsistencyMultiDoc(t *testing.T) {
	// given: 120 objects × 2 docs = 240 docs; a 100-doc round yields only ~50
	// objects, so materializing the 100-object head requires escalation even
	// for a small first page
	fx := NewStoreFixture(t)
	const (
		total    = 120
		pageSize = 15
	)
	objects := make([]TestObject, 0, total)
	batcher := fx.fts.NewAutoBatcher()
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("obj%03d", i)
		title := "apple" + strings.Repeat(" filler", i/4)
		objects = append(objects, TestObject{
			bundle.RelationKeyId:   domain.String(id),
			bundle.RelationKeyName: domain.String(title),
		})
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id:      id + "/r/name",
			SpaceId: "test",
			Title:   title,
		}))
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{
			Id:      id + "/b/block1",
			SpaceId: "test",
			Text:    "apple in a block " + title,
		}))
	}
	_, err := batcher.Finish()
	require.NoError(t, err)
	fx.AddObjects(t, objects)

	query := func(limit, offset int) []string {
		records, err := fx.Query(database.Query{
			SpaceId:   "test",
			TextQuery: "apple",
			Limit:     limit,
			Offset:    offset,
		})
		require.NoError(t, err)
		ids := make([]string, 0, len(records))
		for _, r := range records {
			ids = append(ids, r.Details.GetString(bundle.RelationKeyId))
		}
		return ids
	}

	// when
	full := query(total, 0)
	require.Len(t, full, total)

	var paged []string
	for offset := 0; ; offset += pageSize {
		page := query(pageSize, offset)
		if len(page) == 0 {
			break
		}
		paged = append(paged, page...)
	}

	// then
	assert.Equal(t, full, paged)
}
