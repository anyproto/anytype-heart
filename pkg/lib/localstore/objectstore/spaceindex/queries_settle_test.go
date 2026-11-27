package spaceindex

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// countingFTS records the tantivy searches a query runs: whether each read
// highlights.
type countingFTS struct {
	ftsearch.FTSearch
	mu         sync.Mutex
	highlights []bool
}

func (c *countingFTS) Search(spaceId, query string, limit int, withHighlights bool) ([]*ftsearch.DocumentMatch, error) {
	c.mu.Lock()
	c.highlights = append(c.highlights, withHighlights)
	c.mu.Unlock()
	return c.FTSearch.Search(spaceId, query, limit, withHighlights)
}

func (c *countingFTS) calls() []bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]bool(nil), c.highlights...)
}

// settleFixture indexes one body doc per object, with the text body(i), and
// counts the searches run after it.
func settleFixture(t *testing.T, n int, body func(i int) string) (*StoreFixture, *countingFTS) {
	fx := NewStoreFixture(t)
	objects := make([]TestObject, 0, n)
	batcher := fx.fts.NewAutoBatcher()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("obj%04d", i)
		objects = append(objects, TestObject{
			bundle.RelationKeyId:             domain.String(id),
			bundle.RelationKeyName:           domain.String("Untitled"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		})
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{Id: id + "/b/body", SpaceId: "space1", Text: body(i)}))
	}
	_, err := batcher.Finish()
	require.NoError(t, err)
	fx.AddObjects(t, objects)
	counter := &countingFTS{FTSearch: fx.fts}
	fx.fts = counter
	return fx, counter
}

// pagedSorts are the sorts a paged API search sends: relevance, then the id.
var pagedSorts = database.WithIdTieBreak([]database.SortRequest{{RelationKey: bundle.RelationKey_final_score, Type: model.BlockContentDataviewSort_Desc}})

func ids(records []database.Record) []string {
	out := make([]string, len(records))
	for i, record := range records {
		out[i] = record.Details.GetString(bundle.RelationKeyId)
	}
	return out
}

func TestFulltextSettledCandidates(t *testing.T) {
	t.Run("distinct scores cost one highlighted search", func(t *testing.T) {
		// given: each body holds the word a different number of times
		fx, counter := settleFixture(t, 200, func(i int) string {
			return strings.Repeat("needle ", i+1) + strings.Repeat("hay ", 300-i)
		})
		matches, err := counter.FTSearch.Search("space1", "needle", 300, false)
		require.NoError(t, err)
		scores := map[float64]bool{}
		for _, match := range matches {
			scores[match.Score] = true
		}
		require.Len(t, scores, 200, "the fixture must score every body differently")
		counter.highlights = nil

		// when
		records, err := fx.Query(database.Query{SpaceId: "space1", TextQuery: "needle", Sorts: pagedSorts, Limit: 26})

		// then
		require.NoError(t, err)
		assert.Len(t, records, 26)
		assert.Equal(t, []bool{true}, counter.calls())
	})

	t.Run("a tie across the budget is settled with one highlighted search", func(t *testing.T) {
		// given: every body is the same, so every score ties
		fx, counter := settleFixture(t, 300, func(int) string { return "needle in every body" })
		want := make([]string, 26)
		for i := range want {
			want[i] = fmt.Sprintf("obj%04d", i)
		}

		// when
		records, err := fx.Query(database.Query{SpaceId: "space1", TextQuery: "needle", Sorts: pagedSorts, Limit: 26})

		// then: the 26 lowest ids, read once with highlights, then widened without
		require.NoError(t, err)
		assert.Equal(t, want, ids(records))
		calls := counter.calls()
		require.NotEmpty(t, calls)
		assert.True(t, calls[0])
		assert.NotContains(t, calls[1:], true)
	})

	t.Run("a query without the tie-break keeps one search", func(t *testing.T) {
		// given
		fx, counter := settleFixture(t, 300, func(int) string { return "needle in every body" })

		// when
		records, err := fx.Query(database.Query{
			SpaceId: "space1", TextQuery: "needle", Limit: 26,
			Sorts: []database.SortRequest{{RelationKey: bundle.RelationKey_final_score, Type: model.BlockContentDataviewSort_Desc}},
		})

		// then
		require.NoError(t, err)
		assert.Len(t, records, 26)
		assert.Equal(t, []bool{true}, counter.calls())
	})

	t.Run("an unpaged query keeps every tied match", func(t *testing.T) {
		// given
		fx, _ := settleFixture(t, 150, func(int) string { return "needle in every body" })

		// when
		records, err := fx.Query(database.Query{SpaceId: "space1", TextQuery: "needle", Sorts: pagedSorts})

		// then: the unpaged first round, as without the tie-break
		require.NoError(t, err)
		assert.Len(t, records, ftCandidatesMin)
	})

	t.Run("a batch of unusable docs widens before the fallback", func(t *testing.T) {
		// given: 100 malformed docs outscore the one real match, a body the
		// name and snippet fallback cannot find
		fx := NewStoreFixture(t)
		batcher := fx.fts.NewAutoBatcher()
		for i := 0; i < 100; i++ {
			require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{Id: fmt.Sprintf("junk%03d/x/y", i), SpaceId: "space1", Text: "needle"}))
		}
		require.NoError(t, batcher.UpsertDoc(ftsearch.SearchDoc{Id: "real/b/body", SpaceId: "space1", Text: "needle " + strings.Repeat("hay ", 50)}))
		_, err := batcher.Finish()
		require.NoError(t, err)
		fx.AddObjects(t, []TestObject{{
			bundle.RelationKeyId:             domain.String("real"),
			bundle.RelationKeyName:           domain.String("Untitled"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
		}})

		// when
		records, err := fx.Query(database.Query{SpaceId: "space1", TextQuery: "needle", Limit: 1})

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"real"}, ids(records))
	})
}
