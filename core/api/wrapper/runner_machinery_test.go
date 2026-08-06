package wrapper

// runner_machinery_test.go — the machinery the model never authors, under
// its failure modes: the idempotency identity across dry-run/renumbering,
// key persistence across a FAILED mutation (the harness re-run case), the
// retry loop's error body, and session-state concurrency.

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
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
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, `{"diffStats":{},"dry_run":true}`)
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
			apimodel.V2ObjectRow{Id: "bafyobjA", Name: "Q3 report", Type: "task"}))
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(1, false,
			apimodel.V2ObjectRow{Id: "bafyobjB", Name: "Groceries", Type: "task"}))
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
		// after the retry rewrote the ref to a full id, the session retains
		// the labels; the re-run resolves client-side to the SAME resolved
		// request — the re-stamped LastWrite must match it
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"ambiguous_input","message":"block reference \"e0001\" matches more than one block — use the full block id"}`)
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200,
			`{"version":1,"type":"task","blocks":[{"id":"aaaabbbbccccddddeeee0001","type":"paragraph","text":"a"},{"id":"aaaabbbbccccdddd00e0001f","type":"paragraph","text":"b"}]}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "check_item", args)
		require.NoError(t, err)
		fx.now = fx.now.Add(5 * time.Second)
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)

		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 3, "attempt, retry, replayed re-run")
		assert.Equal(t, "aaaabbbbccccddddeeee0001", firstOp(t, sent[2])["id"],
			"the re-run resolves the label client-side from the retained map")
		assert.Equal(t, sent[1].Header.Get("Idempotency-Key"), sent[2].Header.Get("Idempotency-Key"),
			"the key was re-stamped onto the resolved request identity")
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
