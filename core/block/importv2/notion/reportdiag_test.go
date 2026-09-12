package notion

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/core/block/importv2/report"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// TestRenderReport renders the import report the recorded workspace produces
// and logs it as plain text. Not an assertion test — the report is the page a
// user reads after an import, and the only way to judge it is to read it. Run
// with -v, or set REPORT_OUT to write it to a file:
//
//	REPORT_OUT=/tmp/report.txt go test ./core/block/importv2/notion/ \
//	  -run TestRenderReport -count=1
func TestRenderReport(t *testing.T) {
	if _, err := cassette.Load(workspaceCassette); err != nil {
		t.Skip("no cassette")
	}
	rec, err := recorder.New(workspaceCassette,
		recorder.WithMode(recorder.ModeReplayOnly),
		recorder.WithSkipRequestLatency(true),
		recorder.WithMatcher(cassette.NewDefaultMatcher(cassette.WithIgnoreAuthorization())),
	)
	require.NoError(t, err)
	defer rec.Stop()
	apiClient := client.NewClient("cassette",
		client.WithTransport(&http.Client{Transport: rec, Timeout: time.Minute}),
		client.WithRateLimit(100000),
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, TotalBudget: 30 * time.Second}),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error { return nil }))
	sink := &recordingSink{}
	_, err = converter.Convert(context.Background(), sink)
	require.NoError(t, err)

	names := map[string]string{}
	for _, o := range sink.objects {
		if o.Payload != nil {
			if n := o.Payload.Details.GetString(bundle.RelationKeyName); n != "" {
				names[o.SourceKey] = n
			}
		}
	}
	object := report.Build("Import report — Notion", sink.issues, 0, func(key string) report.Source {
		return report.Source{Name: names[key], Resolved: names[key] != ""}
	})
	blocks := map[string]*model.Block{}
	for _, b := range object.Payload.Blocks {
		blocks[b.Id] = b
	}
	var out strings.Builder
	var walk func(id string, depth int)
	seen := map[string]bool{}
	walk = func(id string, depth int) {
		b := blocks[id]
		if b == nil || seen[id] {
			return
		}
		seen[id] = true
		if text := b.GetText(); text != nil {
			out.WriteString(strings.Repeat("  ", depth) + text.Text + "\n")
		}
		for _, child := range b.ChildrenIds {
			walk(child, depth+1)
		}
	}
	for _, b := range object.Payload.Blocks {
		walk(b.Id, 0)
	}
	if path := os.Getenv("REPORT_OUT"); path != "" {
		require.NoError(t, os.WriteFile(path, []byte(out.String()), 0o644))
	}
	t.Logf("%d blocks, %d lines\n%s", len(object.Payload.Blocks),
		len(strings.Split(out.String(), "\n")), out.String())
}
