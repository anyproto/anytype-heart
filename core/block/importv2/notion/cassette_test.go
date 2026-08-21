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
			// The -1 is the bundled Tag redirect staying space-wide: this
			// workspace has two "Tags" properties (ids Bfgr and yq%7B~), and
			// the second joins the bundled relation instead of minting its own.
			// The +11 on top is property ids becoming database-scoped: Notion
			// only guarantees them unique WITHIN a database, and this
			// workspace's teamspace templates reuse slug ids ("project",
			// "status") across databases, which collapsed unrelated properties
			// onto one relation.
			Objects:        832,
			FileObjects:    41,
			RootCandidates: 13,
			// 832: +2 relations for the two "Place" properties, which now
			// import their address as text instead of being skipped, and
			// -45 empty database rows. Those rows have no name, no value in
			// any column and nothing on them — 45 of them are the blank
			// filler rows of one Notion contact-list template — and every
			// one used to land in the space as another "Untitled" object.
			// Nothing in the workspace references them.
			// 4451: 606 placeholder paragraphs are gone. 435 of them read
			// "Unsupported block (unsupported)" — Notion buttons, which the
			// API refuses to expose and which have no content to stand in
			// for — and 171 read "Unresolved link: Untitled", one per linked
			// database view. Both are reported instead, per page, with a
			// count. Placeholders remain wherever the content really exists
			// and Anytype has no counterpart — and they now keep their
			// children, which is the +18: the notes inside 3 Notion AI
			// transcription blocks (paragraphs, bullets, to-dos) were
			// fetched and then dropped.
			Blocks:       4451,
			MentionMarks: 1468, // +2: mentions inside those recovered notes
			LinkMarks:    62,
			IssuesByCode: map[importv2.IssueCode]int{
				// 261: the 45 skipped rows each report themselves (an info,
				// which the completeness invariant requires and the report
				// rolls up under their database).
				// Before them, 216 from 341: Notion button properties no longer report
				// a loss per row (112 of them) — a button holds no value,
				// so the schema notes it once — and place properties now
				// import instead of being skipped (17).
				importv2.IssueDataLoss:         261,
				importv2.IssueMissingTarget:    171,
				importv2.IssueUnsupportedBlock: 435, // the 3 transcriptions now import
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
