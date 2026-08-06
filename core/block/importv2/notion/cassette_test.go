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
		// Second-chance discovery probes ids the cassette has no interaction
		// for; an unmatched replay surfaces as a retryable transport error,
		// so the default policy (5 attempts, minutes of backoff) would turn
		// each miss into a multi-second stall. One cheap retry is plenty here.
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 2, BaseDelay: 10 * time.Millisecond, MaxDelay: 50 * time.Millisecond, TotalBudget: 30 * time.Second}),
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
			// 864: a property belongs to the database that declares it, so
			// same-named properties in different databases no longer collapse
			// into one relation (762 before that fix). The whole delta is
			// definition objects — relations 140→208, options 178→213 — with
			// content unchanged at 444; "Status" alone appeared in 18 of this
			// workspace's databases, sharing one option pool between them.
			// The last -1 is the bundled Tag redirect staying space-wide: this
			// workspace has two "Tags" properties (ids Bfgr and yq%7B~), and
			// the second joins the bundled relation instead of minting its own.
			Objects:        864,
			FileObjects:    41,
			RootCandidates: 13,
			Blocks:         5039,
			MentionMarks:   1466,
			LinkMarks:      62,
			IssuesByCode: map[importv2.IssueCode]int{
				importv2.IssueDataLoss:         341,
				importv2.IssueMissingTarget:    171,
				importv2.IssueUnsupportedBlock: 438,
				// 10 databases in the recorded workspace match the naive
				// type suggestor (§11.5): Tasks/Notes/People/Projects by
				// name, CRM via email+phone, 5 trackers via due+status
				// (one gained by select→status counting as a status
				// property in the task shape rule).
				importv2.IssueTypeSuggested: 10,
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
