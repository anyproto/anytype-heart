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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
	// The recorded workspace references the same synced-block original from
	// several places per page (nav-bar pattern); each occurrence must get
	// its own block ids.
	assertUniqueBlockIds(t, sink)

	// Fidelity snapshot: replay over the committed cassette is deterministic,
	// so these numbers pin conversion fidelity exactly. Severity-only checks
	// stayed green through wholesale degradations (every block decoding to an
	// "unsupported" placeholder is just Warnings); a drop here is loud.
	// UPDATE the literals when re-recording the cassette.
	if mode == recorder.ModeReplayOnly {
		assert.Equal(t, fidelitySummary{
			Objects:        763,
			FileObjects:    41,
			RootCandidates: 13,
			Blocks:         5039,
			MentionMarks:   1466,
			LinkMarks:      62,
			IssuesByCode: map[importv2.IssueCode]int{
				importv2.IssueDataLoss:         341,
				importv2.IssueMissingTarget:    171,
				importv2.IssueUnsupportedBlock: 438,
				// 9 databases in the recorded workspace match the naive
				// type suggestor (§11.5): Tasks/Notes/People/Projects by
				// name, CRM via email+phone, 4 trackers via due+status.
				importv2.IssueTypeSuggested: 9,
			},
		}, summarizeFidelity(sink))
	}
}

// fidelitySummary is the cassette conversion's measurable shape.
type fidelitySummary struct {
	Objects        int
	FileObjects    int
	RootCandidates int
	Blocks         int
	MentionMarks   int
	LinkMarks      int
	IssuesByCode   map[importv2.IssueCode]int
}

func summarizeFidelity(sink *recordingSink) fidelitySummary {
	summary := fidelitySummary{IssuesByCode: map[importv2.IssueCode]int{}}
	for _, object := range sink.objects {
		summary.Objects++
		if object.File != nil {
			summary.FileObjects++
		}
		if object.IsRootCandidate {
			summary.RootCandidates++
		}
		if object.Payload == nil {
			continue
		}
		summary.Blocks += len(object.Payload.Blocks)
		for _, block := range object.Payload.Blocks {
			text := block.GetText()
			if text == nil || text.Marks == nil {
				continue
			}
			for _, mark := range text.Marks.Marks {
				switch mark.Type {
				case model.BlockContentTextMark_Mention:
					summary.MentionMarks++
				case model.BlockContentTextMark_Link:
					summary.LinkMarks++
				}
			}
		}
	}
	for _, issue := range sink.issues {
		summary.IssuesByCode[issue.Code]++
	}
	return summary
}
