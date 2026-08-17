package wrapper

// runner_machinery_test.go — the machinery the model never authors, under
// its failure modes: the idempotency identity across dry-run/renumbering,
// key persistence across a FAILED mutation (the harness re-run case), the
// retry loop's error body, and session-state concurrency.

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

func TestIdempotencyIdentity(t *testing.T) {
	ctx := context.Background()
	args := map[string]any{"object": "1", "block": "e0001", "checked": true}

	t.Run("a failed mutation persists its key — the harness re-run replays, not double-applies", func(t *testing.T) {
		// given: the CLI delivery — a FileStore, and a first attempt that
		// dies with a 500 AFTER the key was minted (the applied-but-
		// unanswered case the reuse window exists for)
		fx := newFixture(t)
		path := t.TempDir() + "/session.json"
		store := &FileStore{Path: path}
		require.NoError(t, store.Save(&Session{Space: "space1", Handles: []Handle{{N: 1, Id: "bafyobj1"}}}))
		fx.Runner.store = store
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 500, `{"status":500,"code":"internal","message":"boom"}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		// when: invocation 1 fails; invocation 2 is a fresh process over the
		// same session file, at the same clock
		_, err := fx.Run(ctx, "check_item", args)
		require.Error(t, err)
		second := NewRunner(fx.Runner.client, &FileStore{Path: path})
		second.now = fx.Runner.now
		_, err = second.Run(ctx, "check_item", args)
		require.NoError(t, err)

		// then: ONE key across both attempts — C8 can dedupe the re-run
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 2)
		assert.NotEmpty(t, sent[0].Header.Get("Idempotency-Key"))
		assert.Equal(t, sent[0].Header.Get("Idempotency-Key"), sent[1].Header.Get("Idempotency-Key"),
			"the key minted by the failed attempt must survive — dropping it double-applies")
	})

	t.Run("a dry run and its real twin never share a key", func(t *testing.T) {
		// the server hashes method+path+QUERY+body under one key: a reused
		// key across ?dry_run=true and the real request is a hard 409
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, `{"diff_stats":{},"dry_run":true}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		fx.DryRun = true
		_, err := fx.Run(ctx, "check_item", args)
		require.NoError(t, err)
		fx.DryRun = false
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)

		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 2)
		assert.Equal(t, "true", sent[0].Query.Get("dry_run"))
		assert.Empty(t, sent[1].Query.Get("dry_run"))
		assert.NotEqual(t, sent[0].Header.Get("Idempotency-Key"), sent[1].Header.Get("Idempotency-Key"),
			"different query = different request identity = different key")
	})

	t.Run("a re-find that renumbers the handle re-keys the mutation", func(t *testing.T) {
		// SKILL.md teaches this exact loop: find → edit → find → same edit;
		// handle 1 now names ANOTHER object, so the second edit is a new
		// request and must not replay (or 409) under the first one's key
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false,
			v2model.ObjectRow{Id: "bafyobjA", Name: "Q3 report", Type: "task"}))
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false,
			v2model.ObjectRow{Id: "bafyobjB", Name: "Groceries", Type: "task"}))
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobjA", 200, editOKBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobjB", 200, editOKBody)

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "query": "report"})
		require.NoError(t, err)
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)
		_, err = fx.Run(ctx, "find", map[string]any{"space": "space1", "query": "groceries"})
		require.NoError(t, err)
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)

		first := fx.sent("PATCH /v2/spaces/space1/objects/bafyobjA")
		second := fx.sent("PATCH /v2/spaces/space1/objects/bafyobjB")
		require.Len(t, first, 1)
		require.Len(t, second, 1)
		assert.NotEqual(t, first[0].Header.Get("Idempotency-Key"), second[0].Header.Get("Idempotency-Key"),
			"a different resolved target is a different request — fresh key")
	})

	t.Run("an identical re-run after a successful ambiguity retry replays under the same key", func(t *testing.T) {
		// the retry rewrote the ref mid-flight AFTER the key was minted, and
		// the server bound that key to the REWRITTEN body. The re-run
		// computes the pre-rewrite hash, so LastWrite records the rewrite
		// (PriorHash + Rewrites) and the re-run reproduces it — landing on
		// the exact request identity the C8 store cached, instead of 409ing
		// or re-applying under a fresh key.
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"ambiguous_input","message":"block reference \"e0001\" matches more than one block — use the full block id"}`)
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200,
			`{"version":1,"type":"task","blocks":[{"id":"section-intro-e0001","type":"paragraph","text":"a"},{"id":"section-body-f0002","type":"paragraph","text":"b"}]}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "check_item", args)
		require.NoError(t, err)
		fx.now = fx.now.Add(5 * time.Second)
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)

		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 3, "attempt, retry, replayed re-run")
		assert.Equal(t, "section-intro-e0001", firstOp(t, sent[2])["id"],
			"the re-run reproduces the recorded rewrite")
		assert.Equal(t, sent[1].Header.Get("Idempotency-Key"), sent[2].Header.Get("Idempotency-Key"),
			"the key was re-stamped onto the resolved request identity")
	})

	t.Run("a backwards clock step re-keys instead of reviving a stale record", func(t *testing.T) {
		// LastWrite is persisted in the CLI session file, so a backwards
		// clock step (NTP, a session file moved between hosts) puts lw.At in
		// the future. The window check had no lower bound — any negative age
		// passed `< window` — so an arbitrarily old key (and its recorded
		// rewrite) was revived until the clock caught back up.
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "check_item", args)
		require.NoError(t, err)
		fx.now = fx.now.Add(-10 * time.Minute) // the clock steps back
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)

		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 2)
		assert.NotEqual(t, sent[0].Header.Get("Idempotency-Key"), sent[1].Header.Get("Idempotency-Key"),
			"a stamp from the future is not evidence of a recent retry — mint fresh")
	})

	t.Run("the reuse window is judged from ONE clock reading per call", func(t *testing.T) {
		// the window used to be evaluated twice — once by the rewrite-replay
		// gate, once by mutationKey — from two now() readings; ticking
		// across the boundary between them applied the recorded rewrite and
		// then minted a FRESH key for it: a rewritten body under a new
		// identity, which the server applies again instead of replaying.
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"ambiguous_input","message":"block reference \"e0001\" matches more than one block — use the full block id"}`)
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200,
			`{"version":1,"type":"task","blocks":[{"id":"section-intro-e0001","type":"paragraph","text":"a"},{"id":"section-body-f0002","type":"paragraph","text":"b"}]}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		_, err := fx.Run(ctx, "check_item", args) // records the rewrite under its key
		require.NoError(t, err)

		// the boundary straddle: the first reading of the re-run falls just
		// inside the window, every later one just outside
		base := fx.now
		reads := 0
		fx.Runner.now = func() time.Time {
			reads++
			if reads == 1 {
				return base.Add(idempotencyReuseWindow - time.Second)
			}
			return base.Add(idempotencyReuseWindow + time.Second)
		}
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)

		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 3, "attempt, retry, re-run")
		require.Equal(t, "section-intro-e0001", firstOp(t, sent[2])["id"],
			"the recorded rewrite fired on the re-run")
		assert.Equal(t, sent[1].Header.Get("Idempotency-Key"), sent[2].Header.Get("Idempotency-Key"),
			"a body carrying the recorded rewrite must carry the recorded key — never a fresh one")
	})

	t.Run("a second ambiguity rewrite keeps the ORIGINAL pre-rewrite identity", func(t *testing.T) {
		// the chain was single-level: the second rewrite captured lw.Hash —
		// itself already a rewritten hash — as PriorHash, orphaning the
		// original identity; a third identical run then computed the
		// original hash, matched nothing, minted a fresh key and re-applied
		// (reproduced double-apply). PriorHash must stay the FIRST
		// pre-rewrite hash and the rewrites must accumulate.
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		patch := "PATCH /v2/spaces/space1/objects/bafyobj1"
		getPath := "GET /v2/spaces/space1/objects/bafyobj1"
		ambiguous := `{"status":400,"code":"ambiguous_input","message":"block reference matches more than one block — use the full block id"}`
		// run 1: attempt 400s, re-read resolves "e0001" → "section-intro-e0001"
		fx.stub(patch, 400, ambiguous)
		fx.stub(getPath, 200,
			`{"version":1,"type":"task","blocks":[{"id":"section-intro-e0001","type":"paragraph","text":"a"},{"id":"section-body-f0002","type":"paragraph","text":"b"}]}`)
		fx.stub(patch, 200, editOKBody)
		// run 2: the replayed rewritten body goes ambiguous AGAIN (the
		// document moved); re-read resolves onto the longer spelling
		fx.stub(patch, 400, ambiguous)
		fx.stub(getPath, 200,
			`{"version":1,"type":"task","blocks":[{"id":"part-a-section-intro-e0001","type":"paragraph","text":"a"},{"id":"section-body-f0002","type":"paragraph","text":"b"}]}`)
		fx.stub(patch, 200, editOKBody)
		// run 3: an identical re-run must REPLAY the whole chain
		fx.stub(patch, 200, editOKBody)

		_, err := fx.Run(ctx, "check_item", args)
		require.NoError(t, err)
		fx.now = fx.now.Add(5 * time.Second)
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)
		fx.now = fx.now.Add(5 * time.Second)
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)

		sent := fx.sent(patch)
		require.Len(t, sent, 5, "attempt+retry, replay+retry, replayed re-run")
		key := sent[1].Header.Get("Idempotency-Key")
		assert.Equal(t, key, sent[2].Header.Get("Idempotency-Key"))
		assert.Equal(t, key, sent[3].Header.Get("Idempotency-Key"))
		assert.Equal(t, key, sent[4].Header.Get("Idempotency-Key"),
			"the third run replays under the same key — not a fresh re-apply")
		assert.Equal(t, "part-a-section-intro-e0001", firstOp(t, sent[4])["id"],
			"the third run reproduces the WHOLE rewrite chain")
	})
}

func TestRetryLoopKeepsErrorBody(t *testing.T) {
	// a 429 exhausted after 3 attempts must surface the server's C6 text,
	// not a message-less "server answered 429"
	fx := newFixture(t)
	fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
	fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 429,
		`{"status":429,"code":"rate_limited","message":"write budget exhausted — wait a second and retry"}`)

	_, err := fx.Run(context.Background(), "check_item", map[string]any{"object": "1", "block": "b", "checked": true})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "write budget exhausted — wait a second and retry")
	assert.NotContains(t, err.Error(), "server answered 429")
	assert.Len(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), 3, "the full retry budget ran")
}

func TestTranslateOpsErrorRewritesHints(t *testing.T) {
	// Both of locator.go's refusals put the repair in the issue HINT
	// ("…or give the block id"), and edit_text's slot is `block` with no
	// `id` argument under additionalProperties:false — a model following
	// the un-rewritten hint emits a schema-invalid call. deRest rewrites
	// Hint; translateOpsError must too, or the opsVocab locator rows are
	// dead code that never fires (H1). This test fails if the Hint loop is
	// dropped OR if either locator row is removed from opsVocab.
	te := &ToolError{
		Status: 404,
		Code:   "not_found",
		Text:   `"snippet" appears in 0 blocks — retry with id naming one of:`,
		Issues: []v2model.Issue{{
			Path:    "ops[0].find",
			Message: "the find text must appear in exactly one block for the locator to resolve",
			Hint:    "add surrounding text to find until it appears in one block only, or give the block id",
		}},
	}

	got := translateOpsError(te)

	var gotTe *ToolError
	require.ErrorAs(t, got, &gotTe)
	assert.Equal(t, "retry with block naming one of:", gotTe.Text[strings.Index(gotTe.Text, "retry"):])
	assert.Equal(t, "find", gotTe.Issues[0].Path, "the ops[0]. prefix strips from paths")
	assert.Equal(t, "add surrounding text to find until it appears in one block only, or pass block",
		gotTe.Issues[0].Hint, "the hint's repair must speak the tool vocabulary")
	assert.NotContains(t, gotTe.Issues[0].Hint, "block id")
}

func TestConcurrentRunsDoNotRace(t *testing.T) {
	// the long-lived delivery shares one Runner and one MemoryStore across
	// concurrent tool calls; run with -race to make this bite
	fx := newFixture(t)
	fx.stub("GET /v2/spaces/space1/members/me", 200, `{"id":"_participant_space1_acc"}`)
	fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(0, false))

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = fx.Run(context.Background(), "find",
				map[string]any{"space": "space1", "filter": `assignee = "@me"`})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		assert.NoError(t, err, "goroutine %d", i)
	}
}

func TestFileStoreAtomicSave(t *testing.T) {
	// Save must never leave a torn file: the write goes to a temp file that
	// is renamed into place, and no temp litter survives
	dir := t.TempDir()
	store := &FileStore{Path: dir + "/session.json"}
	require.NoError(t, store.Save(&Session{Space: "space1"}))
	got, err := store.Load()
	require.NoError(t, err)
	assert.Equal(t, "space1", got.Space)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temp files left behind")
	assert.Equal(t, "session.json", entries[0].Name())
}

// TestPrepareValuesIsOrderDeterministic. prepareValues ranged its input map,
// so when two keys were both refusable it reported whichever one Go's map
// iteration reached first — a different message per run, on the surface an
// agent reads back and a human debugs from a transcript.
func TestPrepareValuesIsOrderDeterministic(t *testing.T) {
	// given — a space whose key index holds two fold-ambiguous PAIRS, so
	// both inbound keys are refusable and only the ORDER decides which is
	// named. Bundled or single-word keys cannot produce this.
	fx := newFixture(t)
	formats := map[string]string{
		"moodLevel": "text", "mood_level": "text",
		"dueDate": "date", "due_date": "date",
	}
	values := map[string]any{"MoodLevel": "high", "DueDate": "friday"}

	// when / then — 32 runs, because the order was randomized per run
	for i := 0; i < 32; i++ {
		_, err := fx.Runner.prepareValues(context.Background(), &Session{}, "space1", formats, values, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `property key "DueDate"`,
			"the alphabetically first offending key, every run")
	}
}
