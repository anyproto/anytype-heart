package notion

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
)

// TestSyncedOriginalIsFetchedOnce pins the memoisation of synced-block
// originals. A workspace built from Notion templates puts the same synced
// block on every page; re-walking its subtree per reference cost 303 of one
// recorded workspace's 1286 block requests.
func TestSyncedOriginalIsFetchedOnce(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	server := httptest.NewServer(func() http.HandlerFunc {
		handler := scriptedWorkspace(t)
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			hits[r.URL.Path]++
			mu.Unlock()
			handler(w, r)
		}
	}())
	t.Cleanup(server.Close)

	apiClient := client.NewClient("token",
		client.WithBaseURL(server.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	// given — p1 holds two synced blocks (b4, b5) pointing at one original
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, hits["/blocks/orig1/children"], "the original's subtree is fetched once, however many blocks reference it")

	// and the hoisted copies still get their own block ids, or the snapshot
	// would carry one id under two parents
	page := sink.byKey("p1")
	require.NotNil(t, page)
	ids := map[string]int{}
	for _, block := range page.Payload.Blocks {
		ids[block.Id]++
	}
	for id, count := range ids {
		assert.Equal(t, 1, count, "duplicate block id %q", id)
	}
}

// TestPageTallyReportsOncePerKind pins the per-page tally. A template-heavy
// page holds dozens of Notion buttons and linked views; the ledger is capped
// at IssueCap, so repeating one sentence per block spends the diagnostics of
// everything that happens later in the run.
func TestPageTallyReportsOncePerKind(t *testing.T) {
	// given
	tally := newPageTally()
	mctx := mapContext{pageId: "page1", tally: tally}
	sink := &recordingSink{}
	for i := 0; i < 12; i++ {
		mctx.repeated(importv2.Warning(importv2.IssueUnsupportedBlock, "page1", "withheld").About("button"), sink)
	}
	mctx.repeated(importv2.Warning(importv2.IssueUnsupportedBlock, "page1", "withheld").About("ai_block"), sink)

	// when — nothing is reported until the page is done
	assert.Empty(t, sink.issues)
	tally.flush("page1", sink)

	// then — one row per kind, carrying its count, in first-seen order
	require.Len(t, sink.issues, 2)
	assert.Equal(t, "button", sink.issues[0].Subject)
	assert.Equal(t, 12, sink.issues[0].Count)
	assert.Equal(t, "page1", sink.issues[0].SourceKey)
	assert.Equal(t, "ai_block", sink.issues[1].Subject)
	assert.Equal(t, 1, sink.issues[1].Count)

	// and a second flush repeats nothing
	tally.flush("page1", sink)
	assert.Len(t, sink.issues, 2)
}

func TestRepeatedWithoutTallyReportsImmediately(t *testing.T) {
	// given — a caller outside page conversion
	sink := &recordingSink{}
	mapContext{pageId: "page1"}.repeated(importv2.Warning(importv2.IssueDataLoss, "page1", "lost"), sink)

	// then
	require.Len(t, sink.issues, 1)
	assert.Equal(t, 1, sink.issues[0].Occurrences())
}

// TestSyncedOriginalDepthBudget pins that a subtree fetched with less depth
// budget is not reused by a shallower reference. The depth guard cuts a walk
// short; caching that answer under the block id alone would spread one deep
// reference's truncation to every shallow one.
func TestSyncedOriginalDepthBudget(t *testing.T) {
	// given — the same original, first fetched near the depth limit
	var mu sync.Mutex
	fetches := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		fetches++
		mu.Unlock()
		fmt.Fprint(w, `{"results":[{"id":"fresh","type":"paragraph","has_children":false,"paragraph":{"rich_text":[]}}],"has_more":false,"next_cursor":null}`)
	}))
	t.Cleanup(server.Close)
	apiClient := client.NewClient("token", client.WithBaseURL(server.URL), client.WithRateLimit(1000))
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())
	converter.syncedOriginals["orig"] = syncedEntry{depth: maxBlockDepth - 1, blocks: []notionBlock{{Id: "cut-short"}}}

	// when — a shallow caller asks, with more budget than the cached walk had
	seen := map[string]struct{}{}
	blocks, err := converter.syncedOriginal(context.Background(), "orig", 1, seen, &recordingSink{})

	// then — it fetches rather than reusing a possibly truncated answer
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "fresh", blocks[0].Id)
	assert.Equal(t, 1, fetches)

	// and the fresher, deeper-budget answer replaces the cached one
	blocks, err = converter.syncedOriginal(context.Background(), "orig", maxBlockDepth, seen, &recordingSink{})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "fresh", blocks[0].Id, "the cache now holds the walk with the bigger budget")
	assert.Equal(t, 1, fetches, "and that one is reused")
	assert.Contains(t, seen, "fresh", "a cache hit still records the page's ownership set")
}
