package notion

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"

	importv2 "github.com/anyproto/anytype-heart/core/block/importv2"
	"github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
)

const workspaceCassette = "testdata/cassettes/workspace"

// TestCassetteWorkspace replays a recorded crawl of a real Notion workspace
// through the full converter — the real-shape / schema-drift detector.
//
// Re-record against a live workspace (cassettes are scrubbed of the token):
//
//	NOTION_TOKEN=secret_... go test ./core/block/importv2/notion/ -run TestCassetteWorkspace -count=1
//
// The test skips until a cassette is committed; hostile/edge cases are
// covered by the hand-written scripted workspace instead.
func TestCassetteWorkspace(t *testing.T) {
	token := os.Getenv("NOTION_TOKEN")
	mode := recorder.ModeReplayOnly
	if token != "" {
		mode = recorder.ModeRecordOnly
	} else {
		if _, err := os.Stat(workspaceCassette + ".yaml"); err != nil {
			t.Skip("no cassette recorded yet; set NOTION_TOKEN to record one")
		}
		token = "cassette-token"
	}

	scrub := func(interaction *cassette.Interaction) error {
		delete(interaction.Request.Headers, "Authorization")
		return nil
	}
	rec, err := recorder.New(workspaceCassette,
		recorder.WithMode(mode),
		recorder.WithHook(scrub, recorder.AfterCaptureHook),
		recorder.WithSkipRequestLatency(true),
		recorder.WithMatcher(cassette.NewDefaultMatcher(cassette.WithIgnoreAuthorization())),
	)
	require.NoError(t, err)
	defer rec.Stop()

	apiClient := client.NewClient(token,
		client.WithTransport(&http.Client{Transport: rec, Timeout: time.Minute}),
		client.WithRateLimit(1000),
	)
	converter := New(apiClient, client.NewFileFetcher(), stubFactory{}, t.TempDir())

	claims := 0
	require.NoError(t, converter.EnumerateIdentities(context.Background(), func(importv2.IdentityClaim) error {
		claims++
		return nil
	}))
	require.Positive(t, claims, "the recorded workspace must contain objects")

	sink := &recordingSink{}
	rootSpec, err := converter.Convert(context.Background(), sink)
	require.NoError(t, err)
	assert.Equal(t, "Notion Import", rootSpec.CollectionName)
	assert.NotEmpty(t, sink.objects)
	for _, issue := range sink.issues {
		assert.LessOrEqual(t, issue.Severity, importv2.SeverityWarning,
			"a recorded workspace should convert without object errors: %s", issue.Error())
	}
}
