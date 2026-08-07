package wrapper

// tools_smallmodel_test.go — the §8.21 benchmark fixes: edit_text locating
// its block from the find snippet (with the mandatory ambiguity refusals),
// and case-folded type/property keys. Each behavior here was a measured
// small-model failure: both benchmarked models routed around edit_text
// because block was required, and every argument error in the run was a
// naming/capitalisation guess.

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snippetDoc has three text blocks covering every locate case: "Q3" is
// unique to e0001; "budget" appears in e0002 AND e0003 (two candidate
// blocks); "todo" appears only in e0003 but twice there.
const snippetDoc = `{"version":1,"type":"page","properties":{"name":"Plan"},"blocks":[` +
	`{"id":"aaaabbbbccccddddeeee0001","type":"heading1","text":"Q3 planning"},` +
	`{"id":"aaaabbbbccccddddeeee0002","type":"paragraph","text":"the draft budget is due"},` +
	`{"id":"aaaabbbbccccddddeeee0003","type":"paragraph","text":"todo todo budget"}]}`

func TestEditTextLocatesBlock(t *testing.T) {
	ctx := context.Background()

	t.Run("omitted block resolves from a snippet unique in the document", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, snippetDoc)
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		// when: the turn-one call a small model can actually make — no block
		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "find": "Q3", "replace": "Q4",
		})

		// then
		require.NoError(t, err)
		patches := fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1")
		require.Len(t, patches, 1)
		op := firstOp(t, patches[0])
		assert.Equal(t, "aaaabbbbccccddddeeee0001", op["id"], "the located block's full id is sent")
		assert.Equal(t, "Q3", op["find"])
		session, _ := fx.store.Load()
		assert.Equal(t, "aaaabbbbccccddddeeee0001", session.Labels["bafyobj1"]["e0001"],
			"the locate read retains labels — the next call starts resolved")
	})

	t.Run("zero matches refuses and steers to read mode=outline", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, snippetDoc)

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "find": "Q9", "replace": "Q4",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `no block contains "Q9"`)
		assert.Contains(t, err.Error(), "read with mode=outline")
		assert.Contains(t, err.Error(), "markdown source", "the snippet may have missed only because of markup")
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), "a refusal never writes")
	})

	t.Run("several matching blocks refuse and LIST the candidates with context", func(t *testing.T) {
		// the point of the fix: a silent wrong edit is far worse than a
		// refusal — the candidates carry labels the retry can pass as block
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, snippetDoc)

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "find": "budget", "replace": "plan",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `"budget" appears in 2 blocks`)
		assert.Contains(t, err.Error(), "retry with block naming one of")
		assert.Contains(t, err.Error(), "block e0002 (paragraph)")
		assert.Contains(t, err.Error(), "block e0003 (paragraph)")
		assert.Contains(t, err.Error(), "the draft budget is due", "context distinguishes the candidates")
		assert.NotContains(t, err.Error(), "aaaabbbbccccddddeeee", "candidates use labels, never 24-hex ids")
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), "a refusal never writes")
	})

	t.Run("one block but several occurrences gets the more-context refusal", func(t *testing.T) {
		// the existing must-occur-exactly-once rule, enforced during the
		// locate so the model gets the tip without a wasted PATCH
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/objects/bafyobj1", 200, snippetDoc)

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "find": "todo", "replace": "done",
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `found 2 matches for "todo" in block e0003`)
		assert.Contains(t, err.Error(), "provide more context to make the match unique")
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), "a refusal never writes")
	})

	t.Run("an explicit block still skips the locate read entirely", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("PATCH /v2/spaces/space1/objects/bafyobj1", 200, editOKBody)

		_, err := fx.Run(ctx, "edit_text", map[string]any{
			"object": "1", "block": "e0002", "find": "budget", "replace": "plan",
		})

		require.NoError(t, err)
		assert.Empty(t, fx.sent("GET /v2/spaces/space1/objects/bafyobj1"), "no locate read when block is given")
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
			`{"version":1,"kind":"objectType","key":"page","properties":{"name":"Page"},"typeProperties":[]}`)

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

	t.Run("two type keys differing only by case refuse naming both — never a guess", func(t *testing.T) {
		fx := newFixture(t)
		fx.stub("POST /v2/spaces/space1/search", 400,
			`{"status":400,"code":"validation_failed","message":"type \"PAGE\" not found in space \"space1\""}`)
		fx.stub("GET /v2/spaces/space1/types", 200,
			`{"data":[{"key":"page","name":"page"},{"key":"Page","name":"Page"}],"total":2,"offset":0,"limit":500,"has_more":false}`)

		_, err := fx.Run(ctx, "find", map[string]any{"space": "space1", "type": "PAGE"})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `type key "PAGE" matches several keys differing only by case (Page, page)`)
		assert.Len(t, fx.sent("POST /v2/spaces/space1/search"), 1, "no retry on a collision")
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
		assert.Equal(t, "2026-08-07T00:00:00Z", set["dueDate"],
			"the fold precedes the format lookup — the date convenience fires on the folded key")
		assert.Len(t, fx.sent("GET /v2/spaces/space1/properties/status/options"), 1,
			"the A2 guard runs against the folded key")
	})

	t.Run("two property keys differing only by case refuse naming both", func(t *testing.T) {
		fx := newFixture(t)
		fx.seedSession("space1", Handle{N: 1, Id: "bafyobj1"})
		fx.stub("GET /v2/spaces/space1/properties", 200,
			`{"data":[{"key":"status","name":"status","format":"select"},{"key":"Status","name":"Status","format":"select"}],"total":2,"offset":0,"limit":500,"has_more":false}`)

		_, err := fx.Run(ctx, "set_properties", map[string]any{
			"object": "1", "set": map[string]any{"STATUS": "Done"},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), `property key "STATUS" matches several keys differing only by case (Status, status)`)
		assert.Empty(t, fx.sent("PATCH /v2/spaces/space1/objects/bafyobj1"), "never a guess")
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

func TestSnippetContext(t *testing.T) {
	t.Run("short text passes whole", func(t *testing.T) {
		assert.Equal(t, "the Q3 budget", snippetContext("the Q3 budget", "Q3"))
	})
	t.Run("long text windows with ellipses", func(t *testing.T) {
		text := strings.Repeat("a", 100) + " needle " + strings.Repeat("b", 100)
		got := snippetContext(text, "needle")
		assert.True(t, strings.HasPrefix(got, "…"))
		assert.True(t, strings.HasSuffix(got, "…"))
		assert.Contains(t, got, "needle")
		assert.Less(t, len(got), 80)
	})
	t.Run("never slices mid-rune", func(t *testing.T) {
		text := strings.Repeat("é", 40) + "needle" + strings.Repeat("Ω", 40)
		got := snippetContext(text, "needle")
		assert.True(t, strings.HasPrefix(got, "…"))
		for _, r := range got {
			assert.NotEqual(t, '�', r, "excerpt must stay valid UTF-8")
		}
	})
}
