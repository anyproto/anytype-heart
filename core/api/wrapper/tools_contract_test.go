package wrapper

// tools_contract_test.go — the small-model contract fixes from the Phase-5
// review: the bootstrap tool, empty-string semantics, receipts that name
// their target, table row/column labels, the ops→tool error vocabulary,
// describe's failed-option marker, @me format gating, and the single
// property-index fetch.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

func TestSpacesTool(t *testing.T) {
	ctx := context.Background()

	t.Run("lists name and id — the bootstrap the other tools need", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces", 200,
			`{"data":[{"id":"bafyspace1","name":"Work"},{"id":"bafyspace2","name":"Home"}],"total":2,"offset":0,"limit":25,"has_more":false}`)

		result, err := fx.Run(ctx, "spaces", map[string]any{})

		require.NoError(t, err)
		assert.Contains(t, result.Text, "Work — bafyspace1")
		assert.Contains(t, result.Text, "Home — bafyspace2")
		assert.Contains(t, result.Text, "2 spaces")
		assert.Contains(t, result.Text, "pass the id after the dash as space")
		sent := fx.sent("GET /v2/spaces")
		require.Len(t, sent, 1)
		assert.Equal(t, "25", sent[0].Query.Get("limit"), "default limit")
		assert.Empty(t, sent[0].Header.Get("Idempotency-Key"), "a read carries no idempotency key")
	})

	t.Run("truncation steers to a higher limit", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces", 200,
			`{"data":[{"id":"bafyspace1","name":"Work"}],"total":40,"offset":0,"limit":1,"has_more":true}`)

		result, err := fx.Run(ctx, "spaces", map[string]any{"limit": 1})

		require.NoError(t, err)
		assert.Contains(t, result.Text, "40 spaces — showing 1; raise limit for the rest")
	})
}

func TestEmptyStringArgs(t *testing.T) {
	ctx := context.Background()

	t.Run("edit_text with an empty replace deletes the phrase — the call reaches the wire", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "block": "ab3f2", "find": " (draft)", "replace": "",
		})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		assert.Equal(t, " (draft)", op["find"])
		assert.Equal(t, "", op["replace"], "the empty replacement is sent, not rejected")
	})

	t.Run("set_cell with an empty value clears the cell (null on the wire)", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "set_cell", map[string]any{
			"object": "1", "table": "t9d2c", "row": "row2", "col": "col1", "value": "",
		})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		val, present := op["value"]
		assert.True(t, present, "value must be on the wire")
		assert.Nil(t, val, "empty value maps to the op's documented null-clears semantics")
	})

	t.Run("empty is distinct from missing in the error text", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "block": "ab3f2", "replace": "x",
		})
		require.Error(t, err)
		assert.Equal(t, `edit_text needs "find" — exact text to replace, as it appears in the block`, err.Error())

		_, err = fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "block": "ab3f2", "find": "", "replace": "x",
		})
		require.Error(t, err)
		assert.Equal(t, `edit_text: "find" must not be empty — exact text to replace, as it appears in the block`, err.Error(),
			"the argument WAS supplied — the error must not claim it is missing")
	})

	t.Run("no dangling dash when an arg has no description", func(t *testing.T) {
		err := validateArgs(Tool{Name: "x", Args: []Arg{{Name: "flag", Type: ArgBoolean, Required: true}}}, map[string]any{})
		require.Error(t, err)
		assert.Equal(t, `x needs "flag"`, err.Error())
	})
}

func TestMutationReceiptsNameTheTarget(t *testing.T) {
	ctx := context.Background()

	t.Run("the receipt names the object that was written", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Groceries", Type: "task"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200,
			`{"diffStats":{"propertiesChanged":1}}`)

		result, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "remove": map[string]any{"tags": []any{"x"}},
		})

		require.NoError(t, err)
		assert.Equal(t, `ok — "Groceries": 1 properties changed`, result.Text,
			"a silently renumbered handle must be visible in the transcript")
	})

	t.Run("a raw object id echoes as itself", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1")
		fx.stub("PATCH /v2/spaces/space1/objects/bafyzzz", 200, editOKBody)

		result, err := fx.Run(ctx, "check_item", map[string]any{"object": "bafyzzz", "block": "b", "checked": true})

		require.NoError(t, err)
		assert.Equal(t, "ok — bafyzzz: 1 changed", result.Text)
	})

	t.Run("dry run names the target too", func(t *testing.T) {
		fx := newFixture(t)
		fx.DryRun = true
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1", Name: "Plan", Type: "page"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200,
			`{"diffStats":{"blocksChanged":1},"dry_run":true}`)

		result, err := fx.Run(ctx, "check_item", map[string]any{"object": "1", "block": "b", "checked": true})

		require.NoError(t, err)
		assert.Equal(t, `dry run — "Plan": would apply 1 changed`, result.Text)
	})
}

// testTableDoc is a table document as the SERVER now serves it: minted block
// and row ids relabeled server-side; column ids stay full (a column's tail is
// shared with its derived cell ids, so columns structurally never relabel —
// §8.26).
const testTableDoc = `{"version":1,"type":"page","properties":{"name":"Grid"},"blocks":[` +
	`{"id":"12222","type":"paragraph","text":"intro"},` +
	`{"id":"4ff02","type":"table",` +
	`"columns":[{"id":"bbbbccccddddeeee00000c01"},{"id":"bbbbccccddddeeee00000c02"}],` +
	`"rows":[{"id":"00d01","cells":["a","b"]},{"id":"00d02","cells":["c","d"]}]}]}`

func TestTableReadPassThrough(t *testing.T) {
	ctx := context.Background()

	t.Run("a table read serves the server's id vocabulary verbatim", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, testTableDoc)

		result, err := fx.Run(ctx, "read", map[string]any{"object": "1"})

		require.NoError(t, err)
		assert.Equal(t, testTableDoc, result.Text, "labeling is server-side now — nothing to rewrite")
	})

	t.Run("set_cell passes the served row and column spellings through", func(t *testing.T) {
		// a served read mixes spellings by design: row labels are short,
		// column ids stay full — the server resolves each by exact id or
		// unique suffix (resolveTablePart), so the wrapper forwards verbatim
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "set_cell", map[string]any{
			"object": "1", "table": "4ff02", "row": "00d02", "col": "bbbbccccddddeeee00000c01", "value": "done",
		})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		assert.Equal(t, "4ff02", op["tableId"])
		assert.Equal(t, "00d02", op["row"])
		assert.Equal(t, "bbbbccccddddeeee00000c01", op["col"])
	})
}

func TestOpsErrorVocabulary(t *testing.T) {
	ctx := context.Background()

	t.Run("server op-path errors come back in the tool vocabulary", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"not_found","message":"block \"zzz\" not found — GET the object with ?outline=true to list block ids","issues":[{"path":"ops[0].inside","message":"the reference is a suffix of several block ids"}]}`)

		_, err := fx.Run(ctx, "add_blocks", map[string]any{"object": "1", "under": "zzz", "markdown": "- x"})

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "inside", "the op field name is not the tool's vocabulary")
		assert.NotContains(t, err.Error(), "ops[0]")
		assert.NotContains(t, err.Error(), "?outline=true")
		assert.Contains(t, err.Error(), "under: the reference is a suffix")
		assert.Contains(t, err.Error(), "run read with mode=outline")
	})

	t.Run("markdown fragment paths lose the ops prefix", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
			`{"status":400,"code":"validation_failed","message":"the ops would produce an invalid document — no op was applied","issues":[{"path":"ops[0].markdown[2]","message":"text too long"}]}`)

		_, err := fx.Run(ctx, "add_blocks", map[string]any{"object": "1", "markdown": "- x"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "markdown[2]: text too long")
		assert.NotContains(t, err.Error(), "ops[0]")
	})
}

func TestDescribeResilience(t *testing.T) {
	ctx := context.Background()

	t.Run("a failed option listing is marked, not silently optionless", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/types/task", 200,
			`{"version":1,"kind":"objectType","key":"task","properties":{"name":"Task"},"typeProperties":[{"key":"status","name":"Status","format":"select"}]}`)
		fx.stub("GET /v2/spaces/space1/properties/status/options", 503, `oops`)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse(
			v2model.PropertyRow{Key: "status", Name: "Status", Format: "select"}))

		result, err := fx.Run(ctx, "describe", map[string]any{"space": "space1", "type": "task"})

		require.NoError(t, err)
		assert.Contains(t, result.Text, "options: (could not be listed — run describe again before using this property)")
		js, ok := result.JSON.(describeResult)
		require.True(t, ok)
		require.NotEmpty(t, js.Properties)
		assert.True(t, js.Properties[0].OptionsUnavailable)
	})

	t.Run("unknown type steers to the tool vocabulary, not a REST route", func(t *testing.T) {
		// the stubbed 404 is an OLDER server's phrasing (pre-§8.21, no
		// candidate list) — the rewrite must keep covering it; the case-fold
		// listing finds no variant for "tsak" and the error surfaces
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/types/tsak", 404,
			`{"status":404,"code":"not_found","message":"type \"tsak\" not found in space \"space1\" — list available keys with GET /v2/spaces/space1/types"}`)
		fx.stub("GET /v2/spaces/space1/types", 200,
			`{"data":[{"key":"task","name":"Task"}],"total":1,"offset":0,"limit":500,"has_more":false}`)

		_, err := fx.Run(ctx, "describe", map[string]any{"space": "space1", "type": "tsak"})

		require.Error(t, err)
		assert.NotContains(t, err.Error(), "GET /v2/spaces")
		assert.Contains(t, err.Error(), "check the type key (find results show each object's type)")
	})
}

func TestMeSentinelFormatGating(t *testing.T) {
	ctx := context.Background()

	t.Run("@me stays literal on non-object formats", func(t *testing.T) {
		// propertiesBody: status=select, dueDate=date, assignee=objects,
		// tags=multiSelect — a TEXT value literally containing "@me" is data
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200,
			`{"data":[{"key":"notes","name":"Notes","format":"text"},{"key":"assignee","name":"Assignee","format":"objects"}],"total":2,"offset":0,"limit":500,"has_more":false}`)
		fx.stub("GET /v2/spaces/space1/members/me", 200, `{"id":"_participant_space1_acc"}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1",
			"set":    map[string]any{"notes": "@me", "assignee": "@me"},
		})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		set, _ := op["set"].(map[string]any)
		assert.Equal(t, "@me", set["notes"], "text keeps the literal token")
		assert.Equal(t, "_participant_space1_acc", set["assignee"], "object-format keys resolve")
	})
}

func TestPropertyIndexFetchedOnce(t *testing.T) {
	// one set_properties with set+add+remove used to fetch the whole
	// property index three times
	fx := newFixture(t)
	fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
	fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
	fx.stub("GET /v2/spaces/space1/properties/status/options", 200,
		`{"data":[{"name":"Done"}],"total":1,"offset":0,"limit":50,"has_more":false}`)
	fx.stub("GET /v2/spaces/space1/properties/tags/options", 200,
		`{"data":[{"name":"urgent"}],"total":1,"offset":0,"limit":50,"has_more":false}`)
	fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

	_, err := fx.Run(context.Background(), "set_properties", map[string]any{
		"object": "1",
		"set":    map[string]any{"status": "Done"},
		"add":    map[string]any{"tags": []any{"urgent"}},
		"remove": map[string]any{"tags": []any{"old"}},
	})

	require.NoError(t, err)
	sent := fx.sent("GET /v2/spaces/space1/properties")
	require.Len(t, sent, 1, "the property index is fetched once per tool call")
	assert.Equal(t, "500", sent[0].Query.Get("limit"),
		"the page size stays under the server's MaxPageSize — sitting on the boundary 400s if it is lowered")
}

// The old TestLabelsResolveUnderServerRule pinned the wrapper's own
// suffix-label minting to the server's matchBlockRef rule. With client-side
// relabeling retired there is a single labeler — the server's — whose
// label-uniqueness invariant is pinned at the format level
// (anyblockjson.TestExport_MintedShapeRelabeling) and whose resolution is
// pinned by the service's outline/?block= round-trip tests.
