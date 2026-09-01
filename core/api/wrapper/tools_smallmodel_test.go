package wrapper

// tools_smallmodel_test.go — the §8.21 benchmark fixes: edit_text taking a
// snippet instead of a required block (the locate now lives in the SERVER —
// §8.43 moved it down a layer, retiring the wrapper's read-then-patch
// TOCTOU; the wrapper's job is passing the op through id-less and
// re-spelling the server's refusals in the tool register), and case-folded
// type/property keys. Each behavior here was a measured small-model
// failure: both benchmarked models routed around edit_text because block
// was required, and every argument error in the run was a
// naming/capitalisation guess.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The server-shaped locator refusals (locator.go), as the wire serves them.
// The candidate ids are FULL stored ids — the applier's view is the
// canonical document, and a full id is always a valid retry value.
const (
	locatorZeroBody = `{"status":404,"code":"not_found",` +
		`"message":"no block contains \"Q9\" — copy the find text exactly, including inline markup (text is markdown source: ** [ ] etc. count)",` +
		`"issues":[{"path":"ops[0].find","message":"the find text must appear in exactly one block for the locator to resolve",` +
		`"hint":"GET the object with ?outline=true to list them, then copy the text exactly as a read serves it — or give the block id"}]}`
	locatorAmbiguousBody = `{"status":400,"code":"ambiguous_input",` +
		`"message":"\"budget\" appears in 2 blocks — retry with id naming one of:\n  block aaaaaaaaaaaaaaaaaaaae0002 (paragraph): \"the draft budget is due\"\n  block aaaaaaaaaaaaaaaaaaaae0003 (paragraph): \"todo todo budget\"",` +
		`"issues":[{"path":"ops[0].find","message":"the find text appears in 2 blocks — a locator must identify exactly one",` +
		`"hint":"add surrounding text to find until it appears in one block only, or give the block id"}]}`
	locatorMultiOccurrenceBody = `{"status":400,"code":"validation_failed",` +
		`"message":"found 2 matches for \"todo\" in block \"aaaaaaaaaaaaaaaaaaaae0003\" — provide more context to make the match unique, or set \"replace_all\": true",` +
		`"issues":[{"path":"ops[0].find","message":"2 matches in the block's text"}]}`
)

func TestEditTextLocatesBlock(t *testing.T) {
	ctx := context.Background()

	t.Run("omitted block sends the op id-less — the server locates under its lock", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		// when: the turn-one call a small model can actually make — no block
		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "find": "Q3", "replace": "Q4",
		})

		// then: ONE request total — the locate read is gone (it was a
		// read-then-patch TOCTOU: the document could move between the GET
		// and the PATCH; in-API resolution runs under the object lock)
		require.NoError(t, err)
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/objects/bafyobj1"), "no client-side locate read")
		patches := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, patches, 1)
		op := firstOp(t, patches[0])
		assert.NotContains(t, op, "id", "id omitted — find IS the locator, resolved server-side")
		assert.Equal(t, "Q3", op["find"])
	})

	t.Run("zero matches: the server's refusal arrives re-spelled for the tool register", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 404, locatorZeroBody)

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "find": "Q9", "replace": "Q4",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `no block contains "Q9"`)
		assert.Contains(t, err.Error(), "run read (the default mode=full)", "the REST outline steer is re-spelled (§8.34)")
		assert.NotContains(t, err.Error(), "?outline=true", "no REST vocabulary reaches the model")
		assert.Contains(t, err.Error(), "markdown source", "the snippet may have missed only because of markup")
		assert.Contains(t, err.Error(), "or pass block")
	})

	t.Run("several matching blocks: the candidate list arrives with block as the retry slot", func(t *testing.T) {
		// the point of the design: a silent wrong edit is far worse than a
		// refusal — the candidates carry ids the retry can pass as block
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400, locatorAmbiguousBody)

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "find": "budget", "replace": "plan",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `"budget" appears in 2 blocks`)
		assert.Contains(t, err.Error(), "retry with block naming one of", "the op's id slot is re-spelled as block")
		assert.Contains(t, err.Error(), "block aaaaaaaaaaaaaaaaaaaae0002 (paragraph)")
		assert.Contains(t, err.Error(), "the draft budget is due", "context distinguishes the candidates")
		assert.Len(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), 1,
			"a locator ambiguity is not the suffix-collision race — no rewrite retry fires")
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/objects/bafyobj1"),
			"and no re-read either: with id absent there is nothing to rewrite")
	})

	t.Run("one block but several occurrences gets the more-context refusal, replace_all stripped", func(t *testing.T) {
		// the existing must-occur-exactly-once rule, now enforced where the
		// lock is — the server resolves the block and refuses from there;
		// the replace_all escape names an argument edit_text does not take
		// (§8.6), so opsVocab strips it as it always has
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400, locatorMultiOccurrenceBody)

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "find": "todo", "replace": "done",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `found 2 matches for "todo"`)
		assert.Contains(t, err.Error(), "provide more context to make the match unique")
		assert.NotContains(t, err.Error(), "replace_all")
	})

	t.Run("an explicit block goes into the op's id and skips any locating", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "block": "e0002", "find": "budget", "replace": "plan",
		})

		require.NoError(t, err)
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/objects/bafyobj1"))
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		assert.Equal(t, "e0002", op["id"])
	})
}

func TestEditTextReplaceAllHintStripped(t *testing.T) {
	// the server's escape hint names replace_all — an argument edit_text
	// deliberately does not take (§8.6); following it would earn the
	// wrapper's rejection
	fx := newFixture(t)
	fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
	fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 400,
		`{"status":400,"code":"validation_failed","message":"found 2 matches for \"x\" in block \"e0002\" — provide more context to make the match unique, or set \"replace_all\": true"}`)

	_, err := fx.Run(context.Background(), "edit_text", map[string]any{
		"object": "1", "block": "e0002", "find": "x", "replace": "y",
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "provide more context to make the match unique")
	assert.NotContains(t, err.Error(), "replace_all")
}

// typeListBody stubs the space's type index for the case fold.
const typeListBody = `{"data":[{"key":"page","name":"Page"},{"key":"task","name":"Task"}],"total":2,"offset":0,"limit":500,"has_more":false}`

func TestTypeKeyCaseFold(t *testing.T) {
	ctx := context.Background()
	typeNotFound := func(key string) string {
		return `{"status":404,"code":"not_found","message":"type \"` + key + `\" not found in space \"space1\" — known type keys: page, task; did you mean page?"}`
	}

	t.Run("describe retries once with the unique case variant", func(t *testing.T) {
		// given: the benchmark's literal miss — describe {type: "Page"}
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/types/Page", 404, typeNotFound("Page"))
		fx.stub("GET /v2/spaces/space1/types", 200, typeListBody)
		fx.stub("GET /v2/spaces/space1/types/page", 200,
			`{"formatVersion":"2.0","kind":"object_type","properties":{"name":"Page"},"type_settings":{"api_key":"page","property_definitions":[]}}`)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse())

		// when
		result, err := fx.Run(ctx, "describe", map[string]any{"space": "space1", "type": "Page"})

		// then
		require.NoError(t, err)
		assert.Contains(t, result.Text, "type page", "the resolved key is what the receipt names")
		require.Len(t, fx.sent("GET /v2/spaces/space1/types/page"), 1)
	})

	t.Run("find retries once with the folded type", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 400,
			`{"status":400,"code":"validation_failed","message":"type \"Page\" not found in space \"space1\"","issues":[{"path":"/type","message":"unknown type key \"Page\" — known type keys: page, task","hint":"did you mean page?"}]}`)
		fx.stub("GET /v2/spaces/space1/types", 200, typeListBody)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(0, false))

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "type": "Page"})

		require.NoError(t, err)
		posts := fx.sent("POST /v2/spaces/space1/search")
		require.Len(t, posts, 2)
		assert.Equal(t, "Page", bodyJSON(t, posts[0])["type"])
		assert.Equal(t, "page", bodyJSON(t, posts[1])["type"], "the retry carries the folded key")
	})

	t.Run("create retries with the folded type under a fresh idempotency key", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/objects", 400, typeNotFound("Page"))
		fx.stub("GET /v2/spaces/space1/types", 200, typeListBody)
		fx.stub("POST /v2/spaces/space1/objects", 200, `{"id":"bafynew","type":"page"}`)

		result, err := fx.Run(ctx, "create", map[string]any{"space": "space1", "type": "Page", "name": "X"})

		require.NoError(t, err)
		assert.Contains(t, result.Text, "created bafynew (page)", "the receipt shows the resolved type")
		posts := fx.sent("POST /v2/spaces/space1/objects")
		require.Len(t, posts, 2)
		assert.Equal(t, "page", bodyJSON(t, posts[1])["type"])
		assert.NotEqual(t, posts[0].Header.Get("Idempotency-Key"), posts[1].Header.Get("Idempotency-Key"),
			"a folded body is a different resolved request — it must not reuse the failed key (C8)")
	})

	t.Run("a display name for the type retries with the row's key", func(t *testing.T) {
		// D5: reads teach names, so a model hands the NAME back — here one
		// whose spelling is not a case variant of the key at all
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 400,
			`{"status":400,"code":"validation_failed","message":"type \"Meeting note\" not found in space \"space1\""}`)
		fx.stub("GET /v2/spaces/space1/types", 200,
			`{"data":[{"key":"meeting_note","name":"Meeting note"},{"key":"task","name":"Task"}],"total":2,"offset":0,"limit":500,"has_more":false}`)
		fx.stub("POST /v2/spaces/space1/search", 200, searchResponse(0, false))

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "type": "Meeting note"})

		require.NoError(t, err)
		posts := fx.sent("POST /v2/spaces/space1/search")
		require.Len(t, posts, 2)
		assert.Equal(t, "meeting_note", bodyJSON(t, posts[1])["type"],
			"the fold spans the display name — the retry carries the row's key")
	})

	t.Run("two type keys sharing a fold class refuse naming both — never a guess", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 400,
			`{"status":400,"code":"validation_failed","message":"type \"PAGE\" not found in space \"space1\""}`)
		fx.stub("GET /v2/spaces/space1/types", 200,
			`{"data":[{"key":"page","name":"page"},{"key":"Page","name":"Page"}],"total":2,"offset":0,"limit":500,"has_more":false}`)

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "type": "PAGE"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `type key "PAGE" matches several types (Page, page)`)
		assert.Len(t, fx.sent("POST /v2/spaces/space1/search"), 1, "no retry on a collision")
	})

	t.Run("an exact display name cuts through a fold collision", func(t *testing.T) {
		// "Page" folds together with key `page` AND key `Page` — but it IS
		// exactly one row's name, and a name is a better address than a
		// fold guess
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/types/Page", 404, typeNotFound("Page"))
		fx.stub("GET /v2/spaces/space1/types", 200,
			`{"data":[{"key":"page","name":"Page"},{"key":"PAGE","name":"PAGE"}],"total":2,"offset":0,"limit":500,"has_more":false}`)
		fx.stub("GET /v2/spaces/space1/types/page", 200,
			`{"formatVersion":"2.0","kind":"object_type","properties":{"Name":"Page"},"type_settings":{"api_key":"page","property_definitions":[]}}`)
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesResponse())

		_, err := fx.Run(ctx, "describe", map[string]any{"space": "space1", "type": "Page"})

		require.NoError(t, err)
		require.Len(t, fx.sent("GET /v2/spaces/space1/types/page"), 1)
	})

	t.Run("a fold miss surfaces the server's candidate-bearing error", func(t *testing.T) {
		// "page type" (lifted from the prompt's phrasing) has no case
		// variant — the did-you-mean in the server tip is the repair path
		fx := newFixture(t)
		fx.stub("GET /v2/spaces/space1/types/page type", 404, typeNotFound("page type"))
		fx.stub("GET /v2/spaces/space1/types", 200, typeListBody)

		_, err := fx.Run(ctx, "describe", map[string]any{"space": "space1", "type": "page type"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "known type keys: page, task")
		assert.Contains(t, err.Error(), "did you mean page?")
	})
}

func TestPropertyKeyCaseFold(t *testing.T) {
	ctx := context.Background()

	t.Run("Title-Case property keys fold — before format lookup, so conveniences still fire", func(t *testing.T) {
		// given: the benchmark sent "Name" for name; here "Status"/"DueDate"
		// must fold AND keep the option guard and date resolution working
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("GET /v2/spaces/space1/properties/status/options", 200,
			`{"data":[{"name":"Done"}],"total":1,"offset":0,"limit":50,"has_more":false}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1",
			"set":    map[string]any{"Status": "Done", "DueDate": "friday"},
		})

		// then
		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		set, _ := op["set"].(map[string]any)
		assert.Equal(t, "Done", set["status"], "the folded key goes on the wire")
		assert.NotContains(t, set, "Status")
		assert.Equal(t, "2026-08-07T00:00:00Z", set["due_date"],
			"the fold precedes the format lookup — the date convenience fires on the folded key")
		assert.Len(t, fx.sent("GET /v2/spaces/space1/properties/status/options"), 1,
			"the A2 guard runs against the folded key")
	})

	// The wrapper's fold is the FORMAT's fold (anyblockjson.FoldKeyTerm) —
	// not strings.EqualFold, and not the narrower bundle.FoldApiKey. The
	// wrapper teaches display names now (D5), so the fold must bridge every
	// spelling a model plausibly echoes — the api key, the camelCase guess,
	// and the NAME a read served — onto one row. Revert to a case-only or
	// separator-only fold and one of these fails.
	t.Run("a camelCase guess folds onto the served snake_case key", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		// when
		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1",
			"set":    map[string]any{"dueDate": "friday"},
		})

		// then
		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		set, _ := op["set"].(map[string]any)
		assert.NotContains(t, set, "dueDate", "the served spelling goes on the wire")
		assert.Equal(t, "2026-08-07T00:00:00Z", set["due_date"],
			"and the date convenience fires, because the format lookup found the key")
	})

	t.Run("a display name resolves to the row's api key, conveniences intact", func(t *testing.T) {
		// the spelling describe and read serve now — "Due date", spaces and
		// all — must land on the due_date row, or the date convenience goes
		// silent exactly for the vocabulary the wrapper itself teaches
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1",
			"set":    map[string]any{"Due date": "friday"},
		})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		set, _ := op["set"].(map[string]any)
		assert.NotContains(t, set, "Due date", "the row's api key goes on the wire")
		assert.Equal(t, "2026-08-07T00:00:00Z", set["due_date"],
			"the name resolved before the format lookup, so the date convenience fired")
	})

	t.Run("two properties sharing a fold class refuse naming both", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200,
			`{"data":[{"key":"status","name":"status","format":"select"},{"key":"Status","name":"Status","format":"select"}],"total":2,"offset":0,"limit":500,"has_more":false}`)

		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "set": map[string]any{"STATUS": "Done"},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `property key "STATUS" matches several properties (Status, status)`)
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), "never a guess")
	})

	t.Run("an exact display name cuts through a property fold collision", func(t *testing.T) {
		// "Due date" folds together with a stray `duedate` twin — but it IS
		// exactly one row's name, so it resolves instead of refusing
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200,
			`{"data":[{"key":"due_date","name":"Due date","format":"date"},{"key":"duedate","name":"DueDate","format":"text"}],"total":2,"offset":0,"limit":500,"has_more":false}`)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "set": map[string]any{"Due date": "friday"},
		})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		set, _ := op["set"].(map[string]any)
		assert.Equal(t, "2026-08-07T00:00:00Z", set["due_date"])
	})

	t.Run("a key and its case variant given together refuse — no silent overwrite", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)

		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "remove": map[string]any{"Tags": []any{"a"}, "tags": []any{"b"}},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `property "tags" is given more than once`)
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"))
	})

	t.Run("an unknown key still passes through for the server's did-you-mean", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200, propertiesBody)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "set": map[string]any{"prio": "high"},
		})

		require.NoError(t, err)
		op := firstOp(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")[0])
		set, _ := op["set"].(map[string]any)
		assert.Contains(t, set, "prio", "the server's referential layer owns unknown-key steering")
	})
}

// snippetContext's windowing tests moved to the server with the function
// itself (v2/service/locator_test.go TestLocatorContext) — the locate is
// no longer a wrapper concern.
