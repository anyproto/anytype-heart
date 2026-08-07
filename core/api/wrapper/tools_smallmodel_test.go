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
