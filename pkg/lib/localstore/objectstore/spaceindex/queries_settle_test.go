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
// highlights, and how many docs each highlighted doc read asked for.
type countingFTS struct {
	ftsearch.FTSearch
	mu         sync.Mutex
	highlights []bool
	docReads   []int
}

func (c *countingFTS) SearchDocs(spaceId, query string, docIds []string) ([]*ftsearch.DocumentMatch, error) {
	c.mu.Lock()
	c.docReads = append(c.docReads, len(docIds))
	c.mu.Unlock()
	return c.FTSearch.SearchDocs(spaceId, query, docIds)
}

func (c *countingFTS) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.highlights, c.docReads = nil, nil
}

func (c *countingFTS) highlightedDocReads() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.docReads...)
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

	t.Run("weak tied matches read highlights only for the candidates a page can serve", func(t *testing.T) {
		// given: every body ties under the score gate, which needs the
		// highlights of a weak match
		fx, counter := settleFixture(t, 300, func(int) string { return "needle " + strings.Repeat("haystack ", 24) })
		matches, err := counter.FTSearch.Search("space1", "needle absentword", 1, true)
		require.NoError(t, err)
		require.Less(t, matches[0].Score, minFulltextScore, "the fixture must score under the gate")
		counter.reset()
		want := make([]string, 26)
		for i := range want {
			want[i] = fmt.Sprintf("obj%04d", i)
		}

		// when
		records, err := fx.Query(database.Query{SpaceId: "space1", TextQuery: "needle absentword", Sorts: pagedSorts, Limit: 26})

		// then: the wide round reads no highlights, the gate reads them for
		// the re-rank head only
		require.NoError(t, err)
		assert.Equal(t, want, ids(records))
		assert.Equal(t, []bool{true, false}, counter.calls())
		assert.Equal(t, []int{ftRerankPoolSize}, counter.highlightedDocReads())
	})

	t.Run("a tie past the hard limit is kept where the index cuts it", func(t *testing.T) {
		// given: more tied matches than the hard limit reads
		fx, _ := settleFixture(t, ftCandidatesHardLimit+50, func(int) string { return "needle in every body" })
		page := func(limit int) []string {
			records, err := fx.Query(database.Query{SpaceId: "space1", TextQuery: "needle", Sorts: pagedSorts, Limit: limit})
			require.NoError(t, err)
			return ids(records)
		}

		// when
		small, big := page(26), page(1000)

		// then: a full page, the head of a big page's
		require.Len(t, small, 26)
		require.Len(t, big, 1000)
		assert.Equal(t, big[:26], small)
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

// gateStubFTS serves the highlighted docs a lazy score gate reads.
type gateStubFTS struct {
	ftsearch.FTSearch
	highlighted map[string]*ftsearch.DocumentMatch
	reads       [][]string
}

func (f *gateStubFTS) SearchDocs(_, _ string, docIds []string) ([]*ftsearch.DocumentMatch, error) {
	f.reads = append(f.reads, docIds)
	var out []*ftsearch.DocumentMatch
	for _, id := range docIds {
		if doc, ok := f.highlighted[id]; ok {
			out = append(out, doc)
		}
	}
	return out, nil
}

func TestLazyScoreGate(t *testing.T) {
	// weakDocs builds one weak body doc per object, scores falling, every
	// third one without highlight ranges
	weakDocs := func(n int) (plain []*ftsearch.DocumentMatch, highlighted []*ftsearch.DocumentMatch) {
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("obj%02d/b/body", i)
			score := 0.01 - float64(i/2)*0.0001 // pairs tie
			plain = append(plain, &ftsearch.DocumentMatch{ID: id, Score: score})
			doc := &ftsearch.DocumentMatch{ID: id, Score: score}
			if i%3 != 0 {
				doc.Fragments = map[string]*ftsearch.Highlight{"Text": {Text: "needle", Ranges: [][]int{{0, 6}}}}
			}
			highlighted = append(highlighted, doc)
		}
		return plain, highlighted
	}
	resultIds := func(t *testing.T, docs []*ftsearch.DocumentMatch, enforceMinScore bool) []string {
		results, err := GroupFulltextResults(docs, enforceMinScore)
		require.NoError(t, err)
		out := make([]string, len(results))
		for i, result := range results {
			out[i] = result.Path.ObjectId
		}
		return out
	}

	t.Run("it admits the first objects the full gate admits", func(t *testing.T) {
		// given
		plain, highlighted := weakDocs(12)
		fts := &gateStubFTS{highlighted: map[string]*ftsearch.DocumentMatch{}}
		for _, doc := range highlighted {
			fts.highlighted[doc.ID] = doc
		}
		want := resultIds(t, highlighted, true)[:5]
		gate := &lazyScoreGate{target: 5, passes: map[string]bool{}}

		// when
		gated, pending, err := gate.apply(fts, database.Query{SpaceId: "space1", TextQuery: "needle"}, plain)

		// then: the first five, read for the objects up to them only
		require.NoError(t, err)
		assert.Equal(t, want, resultIds(t, gated, false))
		assert.True(t, pending)
		read := 0
		for _, ids := range fts.reads {
			read += len(ids)
		}
		assert.Less(t, read, len(plain))
	})

	t.Run("a target past every object admits what the full gate admits", func(t *testing.T) {
		// given
		plain, highlighted := weakDocs(12)
		fts := &gateStubFTS{highlighted: map[string]*ftsearch.DocumentMatch{}}
		for _, doc := range highlighted {
			fts.highlighted[doc.ID] = doc
		}
		want := resultIds(t, highlighted, true)
		gate := &lazyScoreGate{target: 100, passes: map[string]bool{}}

		// when
		gated, pending, err := gate.apply(fts, database.Query{SpaceId: "space1", TextQuery: "needle"}, plain)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, resultIds(t, gated, false))
		assert.False(t, pending)
	})
}
