package notion

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
)

// TestExpiredUrlRefresh pins §16 item 5: a Notion-signed file URL captured at
// block-fetch time expires before the lazy download runs; the opener re-mints
// it from the owning block and the download succeeds.
func TestExpiredUrlRefresh(t *testing.T) {
	// given — a file host where the old signature is expired and only the
	// re-minted one works.
	var oldHits, newHits atomic.Int64
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("sig") {
		case "new":
			newHits.Add(1)
			fmt.Fprint(w, "fresh-bytes")
		default:
			oldHits.Add(1)
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `<Error><Code>AccessDenied</Code><Message>Request has expired</Message></Error>`)
		}
	}))
	t.Cleanup(fileServer.Close)

	var refreshCalls atomic.Int64
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := r.Method + " " + r.URL.Path
		switch route {
		case "POST /search":
			fmt.Fprint(w, `{"results":[
				{"object":"page","id":"p1","parent":{"type":"workspace","workspace":true},
				 "properties":{"Name":{"type":"title","title":[{"plain_text":"Root","type":"text"}]}}}
			],"has_more":false,"next_cursor":null}`)
		case "GET /pages/p1":
			fmt.Fprint(w, `{"id":"p1","archived":false,
				"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
				"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Root","type":"text"}]}}}`)
		case "GET /blocks/p1/children":
			fmt.Fprintf(w, `{"results":[
				{"id":"m1","type":"image","has_children":false,"image":{"type":"file","file":{"url":"%s/pic.png?sig=old"}}}
			],"has_more":false,"next_cursor":null}`, fileServer.URL)
		case "GET /blocks/m1":
			refreshCalls.Add(1)
			fmt.Fprintf(w, `{"id":"m1","type":"image","has_children":false,"image":{"type":"file","file":{"url":"%s/pic.png?sig=new"}}}`, fileServer.URL)
		default:
			t.Errorf("unexpected api call: %s", route)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(apiServer.Close)

	apiClient := client.NewClient("token",
		client.WithBaseURL(apiServer.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	var file *importv2.Object
	for _, o := range sink.objects {
		if o.File != nil {
			file = o
		}
	}
	require.NotNil(t, file, "the image block must emit a file object")

	// when — the lazy download runs after the signature expired
	reader, err := file.File.Open(context.Background())

	// then
	require.NoError(t, err, "expired URL must be re-minted, not failed")
	defer reader.Close()
	content, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, "fresh-bytes", string(content))
	assert.Equal(t, int64(1), refreshCalls.Load(), "exactly one block re-fetch")
	assert.Equal(t, int64(1), oldHits.Load())
	assert.Equal(t, int64(1), newHits.Load())
}

// TestExpiredUrlNoRefreshForExternal pins the negative: external URLs are not
// signed, so a 403 is a real denial and must not trigger a block re-fetch
// (the API fake reports any unexpected call).
func TestExpiredUrlNoRefreshForExternal(t *testing.T) {
	fileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(fileServer.Close)

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		route := r.Method + " " + r.URL.Path
		switch route {
		case "POST /search":
			fmt.Fprint(w, `{"results":[
				{"object":"page","id":"p1","parent":{"type":"workspace","workspace":true},
				 "properties":{"Name":{"type":"title","title":[{"plain_text":"Root","type":"text"}]}}}
			],"has_more":false,"next_cursor":null}`)
		case "GET /pages/p1":
			fmt.Fprint(w, `{"id":"p1","archived":false,
				"created_time":"2024-02-01T10:00:00.000Z","last_edited_time":"2024-02-02T10:00:00.000Z",
				"properties":{"Name":{"id":"title","type":"title","title":[{"plain_text":"Root","type":"text"}]}}}`)
		case "GET /blocks/p1/children":
			fmt.Fprintf(w, `{"results":[
				{"id":"m1","type":"image","has_children":false,"image":{"type":"external","external":{"url":"%s/pic.png"}}}
			],"has_more":false,"next_cursor":null}`, fileServer.URL)
		default:
			t.Errorf("unexpected api call: %s", route)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(apiServer.Close)

	apiClient := client.NewClient("token",
		client.WithBaseURL(apiServer.URL),
		client.WithRateLimit(1000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))
	sink := &recordingSink{}
	_, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	var file *importv2.Object
	for _, o := range sink.objects {
		if o.File != nil {
			file = o
		}
	}
	require.NotNil(t, file)

	// when / then — the failure surfaces without any API refresh call
	_, err = file.File.Open(context.Background())
	require.Error(t, err)
}
