package notion

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// TestPrefetchParallelism pins the page-fetch pipeline: fetches overlap (v1
// parity — its 10-worker pool; serial fetching was RTT-bound below the rate
// allowance) while emission order stays the deterministic stub order.
func TestPrefetchParallelism(t *testing.T) {
	const pageCount = 12

	// given — a workspace of pages whose responses are slow enough that a
	// serial fetcher could never overlap two requests.
	var inFlight, maxInFlight atomic.Int64
	track := func() func() {
		now := inFlight.Add(1)
		for {
			max := maxInFlight.Load()
			if now <= max || maxInFlight.CompareAndSwap(max, now) {
				break
			}
		}
		return func() { inFlight.Add(-1) }
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/search" {
			results := ""
			for i := 0; i < pageCount; i++ {
				if i > 0 {
					results += ","
				}
				results += fmt.Sprintf(`{"object":"page","id":"p%02d","parent":{"type":"workspace","workspace":true},
					"properties":{"Name":{"type":"title","title":[{"plain_text":"Page %02d","type":"text"}]}}}`, i, i)
			}
			fmt.Fprintf(w, `{"results":[%s],"has_more":false,"next_cursor":null}`, results)
			return
		}
		done := track()
		defer done()
		time.Sleep(30 * time.Millisecond)
		var id string
		if n, _ := fmt.Sscanf(r.URL.Path, "/pages/p%s", &id); n == 1 {
			fmt.Fprintf(w, `{"id":"p%s","archived":false,
				"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
				"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Page %s","type":"text"}]}}}`, id, id)
			return
		}
		if n, _ := fmt.Sscanf(r.URL.Path, "/blocks/p%s/children", &id); n == 1 && len(id) >= 2 {
			fmt.Fprintf(w, `{"results":[
				{"id":"b%s","type":"paragraph","has_children":false,"paragraph":{"rich_text":[{"plain_text":"body","type":"text"}]}}
			],"has_more":false,"next_cursor":null}`, id[:2])
			return
		}
		t.Errorf("unexpected api call: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	apiClient := client.NewClient("token",
		client.WithBaseURL(server.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))

	// when
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	// then — fetches overlapped, emission stayed in stub order
	assert.GreaterOrEqual(t, maxInFlight.Load(), int64(2),
		"page fetches must overlap; serial fetching is RTT-bound below the rate allowance")
	assert.LessOrEqual(t, maxInFlight.Load(), int64(prefetchInFlight),
		"in-flight fetches must respect the pipeline bound")
	var emitted []string
	for _, o := range sink.objects {
		emitted = append(emitted, o.Payload.Details.GetString(bundle.RelationKeyName))
	}
	want := make([]string, 0, pageCount)
	for i := 0; i < pageCount; i++ {
		want = append(want, fmt.Sprintf("Page %02d", i))
	}
	assert.Equal(t, want, emitted, "emission order must stay the deterministic stub order")
}
