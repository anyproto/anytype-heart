package notion

import (
	"context"
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
