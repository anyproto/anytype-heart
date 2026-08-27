package wrapper

// tools_write_test.go — the mutation tools: wire shapes, the value
// conveniences (@me, relative dates, the option guard), the idempotency
// machinery and the ambiguity retry.

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

// propertiesBody stubs the space's property index (formats).
const propertiesBody = `{"data":[{"key":"status","name":"Status","format":"select"},{"key":"due_date","name":"Due date","format":"date"},{"key":"assignee","name":"Assignee","format":"objects"},{"key":"tags","name":"Tags","format":"multiSelect"}],"total":4,"offset":0,"limit":1000,"has_more":false}`

func TestCreate(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves @me, relative dates and option names into the shortcut body", func(t *testing.T) {
		// given: 2026-08-06 is a Thursday, so "friday" = 2026-08-07
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("GET /v2/spaces/space1/members/me", 200, `{"id":"_participant_space1_acc"}`)
		fx.stub("GET /v2/spaces/space1/properties/status/options", 200,
			`{"data":[{"name":"Done"}],"total":1,"offset":0,"limit":50,"has_more":false}`)
		fx.stub("POST /v2/spaces/space1/objects", 200, `{"id":"bafynew","type":"task","etag":"e1"}`)

		// when
		result, err := fx.Run(ctx, "create", map[string]any{
			"space": "space1", "type": "task", "name": "Report",
			"properties": map[string]any{"status": "Done", "due_date": "friday", "assignee": "@me"},
			"markdown":   "# Plan\n\n- [ ] draft",
		})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "created bafynew (task)")
		sent := fx.sent("POST /v2/spaces/space1/objects")
		require.Len(t, sent, 1)
		assert.NotEmpty(t, sent[0].Header.Get("Idempotency-Key"), "mutations carry an Idempotency-Key")
		assert.Empty(t, sent[0].Header.Get("If-Match"), "the task tools never send If-Match")
		body := bodyJSON(t, sent[0])
		assert.Equal(t, "task", body["type"])
		assert.Equal(t, "# Plan\n\n- [ ] draft", body["markdown"], "markdown passes through — the server parses")
		props, _ := body["properties"].(map[string]any)
		assert.Equal(t, "Done", props["status"])
		assert.Equal(t, "_participant_space1_acc", props["assignee"], "@me resolves to the caller's participant id")
		assert.Equal(t, "2026-08-07T00:00:00Z", props["due_date"], "friday resolves to the next Friday at midnight")
	})

	t.Run("unknown option name is blocked before the server (the A2 guard)", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("GET /v2/spaces/space1/properties/status/options", 200,
			`{"data":[],"total":0,"offset":0,"limit":50,"has_more":false}`)
		fx.stub("GET /v2/spaces/space1/properties/status/options", 200,
			`{"data":[{"name":"Backlog"},{"name":"Done"}],"total":2,"offset":0,"limit":15,"has_more":false}`)

		_, err := fx.Run(ctx, "create", map[string]any{
			"space": "space1", "type": "task", "name": "X",
			"properties": map[string]any{"status": "done"},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `property "status" has no option named "done" — this tool never creates options`)
		assert.Contains(t, err.Error(), "existing: Backlog, Done")
		assert.Contains(t, err.Error(), `did you mean "Done"?`)
		assert.Empty(t, fx.sent("POST /v2/spaces/space1/objects"), "nothing reaches the create endpoint")
	})

	t.Run("AllowNewOptions bypasses the guard (R9 create-missing stands on REST)", func(t *testing.T) {
		fx := newFixture(t)
		fx.AllowNewOptions = true
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("POST /v2/spaces/space1/objects", 200, `{"id":"bafynew","type":"task"}`)

		_, err := fx.Run(ctx, "create", map[string]any{
			"space": "space1", "type": "task", "name": "X",
			"properties": map[string]any{"status": "Brand new"},
		})

		require.NoError(t, err)
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/properties/status/options"))
	})

	t.Run("dry run flag rides the query", func(t *testing.T) {
		fx := newFixture(t)
		fx.DryRun = true
		fx.stub("POST /v2/spaces/space1/objects", 200, `{"type":"task","dry_run":true}`)

		result, err := fx.Run(ctx, "create", map[string]any{"space": "space1", "type": "task", "name": "X"})

		require.NoError(t, err)
		assert.Contains(t, result.Text, "dry run")
		sent := fx.sent("POST /v2/spaces/space1/objects")
		require.Len(t, sent, 1)
		assert.Equal(t, "true", sent[0].Query.Get("dry_run"))
	})
}

func TestSetProperties(t *testing.T) {
	ctx := context.Background()

	t.Run("set and add become one set_properties op", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("GET /v2/spaces/space1/properties/status/options", 200,
			`{"data":[{"name":"Done"}],"total":1,"offset":0,"limit":50,"has_more":false}`)
		fx.stub("GET /v2/spaces/space1/properties/tags/options", 200,
			`{"data":[{"name":"urgent"}],"total":1,"offset":0,"limit":50,"has_more":false}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200,
			`{"diff_stats":{"blocks_added":0,"blocks_removed":0,"blocks_changed":0,"blocks_moved":0,"properties_changed":2}}`)

		// when
		result, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1",
			"set":    map[string]any{"status": "Done"},
			"add":    map[string]any{"tags": []any{"urgent"}},
		})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "2 properties changed")
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 1)
		assert.NotEmpty(t, sent[0].Header.Get("Idempotency-Key"))
		op := firstOp(t, sent[0])
		assert.Equal(t, "set_properties", op["op"])
		assert.Equal(t, map[string]any{"status": "Done"}, op["set"], "scalar stays scalar — the server coerces list shapes")
		assert.Equal(t, map[string]any{"tags": []any{"urgent"}}, op["add"])
	})

	t.Run("remove skips the option guard", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1",
			"remove": map[string]any{"tags": []any{"gone-name"}},
		})

		require.NoError(t, err)
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/properties/tags/options"),
			"remove never creates anything — no guard fetch")
	})

	t.Run("empty call steers", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		_, err := fx.Run(ctx, "set_properties", map[string]any{"object": "1"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "set_properties needs set, add or remove")
	})
}

func TestBlockTools(t *testing.T) {
	ctx := context.Background()

	// refs pass through verbatim: the server labels reads itself and
	// resolves a label by exact id or unique suffix on every write channel
	// (C4) — the wrapper's session label map is retired.
	seed := func(fx *fixture) {
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
	}

	t.Run("check_item passes the served label through and sends update_block", func(t *testing.T) {
		fx := newFixture(t)
		seed(fx)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		result, err := fx.Run(ctx, "check_item", map[string]any{"object": "1", "block": "e0001", "checked": true})

		require.NoError(t, err)
		assert.Contains(t, result.Text, "1 changed")
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		assert.Equal(t, "update_block", op["op"])
		assert.Equal(t, "e0001", op["id"], "the ref goes to the server as the model spoke it — resolution is server-side")
		assert.Equal(t, map[string]any{"checked": true}, op["set"])
	})

	t.Run("add_blocks sends the markdown channel with under→inside", func(t *testing.T) {
		fx := newFixture(t)
		seed(fx)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "add_blocks", map[string]any{
			"object": "1", "under": "e0001", "markdown": "- [ ] follow up",
		})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		assert.Equal(t, "insert_blocks", op["op"])
		assert.Equal(t, "- [ ] follow up", op["markdown"])
		assert.Equal(t, "e0001", op["inside"], "under maps to the op's inside, verbatim")
		assert.NotContains(t, op, "blocks")
	})

	t.Run("add_blocks with both anchors is a wrapper-side error", func(t *testing.T) {
		fx := newFixture(t)
		seed(fx)
		_, err := fx.Run(ctx, "add_blocks", map[string]any{
			"object": "1", "after": "a", "under": "b", "markdown": "x",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "give after or under, not both")
	})

	t.Run("edit_text sends replace_text without replace_all", func(t *testing.T) {
		fx := newFixture(t)
		seed(fx)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "block": "e0001", "find": "Q3", "replace": "Q4",
		})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		assert.Equal(t, "replace_text", op["op"])
		assert.Equal(t, "Q3", op["find"])
		assert.Equal(t, "Q4", op["replace"])
		assert.NotContains(t, op, "replace_all", "single-match only for the small tier")
	})

	t.Run("set_cell sends the flat cell write", func(t *testing.T) {
		fx := newFixture(t)
		seed(fx)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "set_cell", map[string]any{
			"object": "1", "table": "e0001", "row": "row2", "col": "col1", "value": "done",
		})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		assert.Equal(t, "set_cell", op["op"])
		assert.Equal(t, "e0001", op["table_id"])
		assert.Equal(t, "row2", op["row"])
		assert.Equal(t, "col1", op["col"])
		assert.Equal(t, "done", op["value"])
	})

	t.Run("move_block root-append omits anchors", func(t *testing.T) {
		fx := newFixture(t)
		seed(fx)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "move_block", map[string]any{"object": "1", "block": "e0001"})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		assert.Equal(t, "move_block", op["op"])
		assert.NotContains(t, op, "after")
		assert.NotContains(t, op, "inside")
	})

	t.Run("delete_block forwards recursive and the server's counting error", func(t *testing.T) {
		fx := newFixture(t)
		seed(fx)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"validation_failed","message":"block \"e0001\" has 2 descendant blocks — pass \"recursive\": true to delete the whole subtree"}`)

		_, err := fx.Run(ctx, "delete_block", map[string]any{"object": "1", "block": "e0001"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `pass "recursive": true`)
	})
}

func TestAmbiguityRetry(t *testing.T) {
	ctx := context.Background()

	// Scope, honestly stated: refs go to the server verbatim, so a 400
	// ambiguous_input means the ref did not resolve against the document
	// the server saw. The retry pool is the re-read document's own SERVED
	// ids — never a session map, which could be stale — so the retry fires
	// exactly when the ref uniquely tails a served id on re-read: the
	// concurrent-modification race, where the collision the server saw at
	// PATCH time is gone by the re-read. A persistent ambiguity is
	// unresolvable in principle and surfaces the server's error untouched
	// (the second subtest).

	t.Run("a raced ambiguity re-reads and retries once with the now-unique served id", func(t *testing.T) {
		// given: at PATCH time the server's document had a second block
		// whose id ended in e0001 (the 400 below); by the re-read that block
		// is gone. The surviving ids are meaningful (dashed), so the server
		// serves them in full and the ref now tails exactly one of them.
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"ambiguous_input","message":"block reference \"e0001\" matches more than one block — use the full id"}`)
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200,
			`{"version":1,"type":"task","blocks":[{"id":"section-intro-e0001","type":"paragraph","text":"a"},{"id":"section-body-f0002","type":"paragraph","text":"b"}]}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		// when
		_, err := fx.Run(ctx, "check_item", map[string]any{"object": "1", "block": "e0001", "checked": true})

		// then
		require.NoError(t, err)
		patches := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, patches, 2, "one ambiguous attempt, one retry")
		assert.Equal(t, "e0001", firstOp(t, patches[0])["id"])
		assert.Equal(t, "section-intro-e0001", firstOp(t, patches[1])["id"],
			"the retry uses the re-read's served spelling")
		assert.Equal(t, patches[0].Header.Get("Idempotency-Key"), patches[1].Header.Get("Idempotency-Key"),
			"the retry keeps the same idempotency key")
	})

	t.Run("a still-unresolvable ambiguity surfaces the server's error", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"ambiguous_input","message":"block reference \"0001\" matches more than one block — use the full id"}`)
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200,
			`{"version":1,"type":"task","blocks":[{"id":"chapter-a-0001","type":"paragraph"},{"id":"chapter-b-0001","type":"paragraph"}]}`)

		_, err := fx.Run(ctx, "check_item", map[string]any{"object": "1", "block": "0001", "checked": true})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "matches more than one block")
		assert.Len(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), 1, "no blind retry")
	})
}

func TestIdempotencyMachinery(t *testing.T) {
	ctx := context.Background()
	args := map[string]any{"object": "1", "block": "e0001", "checked": true}

	t.Run("an identical call within the window reuses the key", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "check_item", args)
		require.NoError(t, err)
		fx.now = fx.now.Add(10 * time.Second)
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)

		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 2)
		assert.Equal(t, sent[0].Header.Get("Idempotency-Key"), sent[1].Header.Get("Idempotency-Key"),
			"a quick identical repeat is a retry — same key, C8 replays it")
	})

	t.Run("after the window an identical call is intentional — fresh key", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "check_item", args)
		require.NoError(t, err)
		fx.now = fx.now.Add(2 * time.Minute)
		_, err = fx.Run(ctx, "check_item", args)
		require.NoError(t, err)

		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 2)
		assert.NotEqual(t, sent[0].Header.Get("Idempotency-Key"), sent[1].Header.Get("Idempotency-Key"))
	})

	t.Run("a transient 503 retries the same body and key", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 503, `oops`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "check_item", args)

		require.NoError(t, err)
		sent := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, sent, 2)
		assert.Equal(t, sent[0].Body, sent[1].Body, "the exact body is resent")
		assert.Equal(t, sent[0].Header.Get("Idempotency-Key"), sent[1].Header.Get("Idempotency-Key"))
	})
}

func TestRelativeDates(t *testing.T) {
	// 2026-08-06 is a Thursday
	now := time.Date(2026, 8, 6, 15, 30, 0, 0, time.UTC)
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"today", "2026-08-06T00:00:00Z", true},
		{"Tomorrow", "2026-08-07T00:00:00Z", true},
		{"yesterday", "2026-08-05T00:00:00Z", true},
		{"friday", "2026-08-07T00:00:00Z", true},
		{"thursday", "2026-08-06T00:00:00Z", true},
		{"wednesday", "2026-08-12T00:00:00Z", true},
		{"+3d", "2026-08-09T00:00:00Z", true},
		{"-2d", "2026-08-04T00:00:00Z", true},
		{"2026-08-01", "", false},
		{"next sprint", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, ok := resolveRelativeDate(tt.input, now)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestFileStore(t *testing.T) {
	t.Run("round trip", func(t *testing.T) {
		path := t.TempDir() + "/session.json"
		store := &FileStore{Path: path}
		want := &Session{Space: "space1", Handles: []Handle{{N: 1, Id: "obj1", Name: "A", Type: "task"}}}
		require.NoError(t, store.Save(want))
		got, err := store.Load()
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
	t.Run("missing and corrupt files start fresh", func(t *testing.T) {
		store := &FileStore{Path: t.TempDir() + "/none.json"}
		got, err := store.Load()
		require.NoError(t, err)
		assert.Equal(t, &Session{}, got)

		path := t.TempDir() + "/bad.json"
		require.NoError(t, (&FileStore{Path: path}).Save(&Session{Space: "s"}))
		require.NoError(t, writeFile(path, "{corrupt"))
		got, err = (&FileStore{Path: path}).Load()
		require.NoError(t, err)
		assert.Equal(t, &Session{}, got)
	})
}
