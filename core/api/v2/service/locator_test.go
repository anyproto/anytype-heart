package v2service

// locator_test.go — unit pins for resolveByText (Wave 2.1a/2.1b, §8.43,
// §8.45) that the service-level tests in edit_test.go cannot reach: the
// candidate cap (documents with >8 matching blocks) and the per-op scope
// (the real format never puts `text` on a non-text block, so the scopes are
// unobservable through PatchObject — they are the invariants that a block
// replaceText cannot edit never captures or contaminates ITS match, while
// nothing is ever filtered out of a destructive op's candidate set).
// locatorContext's windowing tests moved here from the wrapper with the
// function itself.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// findDoc builds a v2EditDoc straight from block objects.
func findDoc(t *testing.T, blocks ...string) *v2EditDoc {
	t.Helper()
	doc, err := parseEditDoc([]byte(`{"blocks":[` + strings.Join(blocks, ",") + `]}`))
	require.NoError(t, err)
	return doc
}

func TestResolveByText(t *testing.T) {
	t.Run("the candidate list caps at 8 and counts the rest", func(t *testing.T) {
		blocks := make([]string, 10)
		for i := range blocks {
			blocks[i] = fmt.Sprintf(`{"id":"blk%02d","type":"paragraph","text":"needle %d"}`, i, i)
		}
		doc := findDoc(t, blocks...)

		_, err := resolveByText(doc, "needle", "find", "ops[0].find", textBlocksOnly)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, `"needle" appears in 10 blocks`)
		assert.Contains(t, apiErr.Message, "block blk07 (paragraph)", "the 8th candidate is listed")
		assert.NotContains(t, apiErr.Message, "blk08", "the 9th is not")
		assert.Contains(t, apiErr.Message, "… and 2 more")
	})

	t.Run("a non-text block never captures replaceText's match", func(t *testing.T) {
		// were a non-text block to carry the snippet, resolving to it would
		// produce an op the applier must then refuse — so it must not count
		doc := findDoc(t,
			`{"id":"bmOne1","type":"bookmark","text":"needle"}`,
			`{"id":"parOne1","type":"paragraph","text":"needle"}`)

		idx, err := resolveByText(doc, "needle", "find", "ops[0].find", textBlocksOnly)

		require.NoError(t, err, "the paragraph is the unique TEXT match — no ambiguity")
		assert.Equal(t, 1, idx)
	})

	t.Run("a snippet found only in a non-text block is zero matches for replaceText", func(t *testing.T) {
		doc := findDoc(t,
			`{"id":"bmOne1","type":"bookmark","text":"needle"}`,
			`{"id":"parOne1","type":"paragraph","text":"other"}`)

		_, err := resolveByText(doc, "needle", "find", "ops[0].find", textBlocksOnly)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeNotFound, apiErr.Code)
	})

	t.Run("code and embed text participates (§8.4 literal channels)", func(t *testing.T) {
		doc := findDoc(t,
			`{"id":"parOne1","type":"paragraph","text":"prose"}`,
			`{"id":"codeOne1","type":"code","text":"var needle int"}`)

		idx, err := resolveByText(doc, "needle", "find", "ops[0].find", textBlocksOnly)

		require.NoError(t, err)
		assert.Equal(t, 1, idx)
	})

	// ---- the everyBlock scope (2.1b): nothing is filtered out of a
	// destructive op's candidate set ----

	t.Run("a non-text block IS a candidate for match, so two candidates refuse", func(t *testing.T) {
		// the same document replaceText resolves uniquely above. deleteBlock
		// can delete a bookmark, so filtering it out would answer a genuinely
		// two-block question with one silent, wrong, destructive match. The
		// exporter never writes `text` on a bookmark today — this fixture is
		// synthetic on purpose: it is the only way to state which scope each
		// op has before a format change makes it observable.
		doc := findDoc(t,
			`{"id":"bmOne1","type":"bookmark","text":"needle"}`,
			`{"id":"parOne1","type":"paragraph","text":"needle"}`)

		_, err := resolveByText(doc, "needle", "match", "ops[0].match", everyBlock)

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, "block bmOne1 (bookmark)")
		assert.Contains(t, apiErr.Message, "block parOne1 (paragraph)")
	})

	t.Run("the refusals name the field that carried the text", func(t *testing.T) {
		// the repair has to speak the caller's own vocabulary: an updateBlock
		// told to "add surrounding text to find" is told to edit a field it
		// does not have
		doc := findDoc(t, `{"id":"parOne1","type":"paragraph","text":"other"}`)

		_, err := resolveByText(doc, "needle", "match", "ops[0].match", everyBlock)

		apiErr := v2Err(t, err)
		assert.Contains(t, apiErr.Message, "copy the match text exactly")
		require.Len(t, apiErr.Issues, 1)
		assert.Contains(t, apiErr.Issues[0].Message, "the match text must appear in exactly one block")
		assert.NotContains(t, apiErr.Message, "find")
	})

	t.Run("repeats within the one block still resolve it", func(t *testing.T) {
		// within-block multiplicity is not a resolution failure: this
		// function identifies a BLOCK, and updateBlock/deleteBlock act on the
		// block as a whole. Only replaceText, which has to splice one
		// occurrence, refuses on the count (applyReplaceText)
		doc := findDoc(t,
			`{"id":"parOne1","type":"paragraph","text":"the Q3 report and the Q3 plan"}`,
			`{"id":"parTwo2","type":"paragraph","text":"unrelated"}`)

		idx, err := resolveByText(doc, "Q3", "match", "ops[0].match", everyBlock)

		require.NoError(t, err)
		assert.Equal(t, 0, idx)
	})
}

func TestLocatorContext(t *testing.T) {
	t.Run("short text passes whole", func(t *testing.T) {
		assert.Equal(t, "the Q3 budget", locatorContext("the Q3 budget", "Q3"))
	})
	t.Run("long text windows with ellipses", func(t *testing.T) {
		text := strings.Repeat("a", 100) + " needle " + strings.Repeat("b", 100)
		got := locatorContext(text, "needle")
		assert.True(t, strings.HasPrefix(got, "…"))
		assert.True(t, strings.HasSuffix(got, "…"))
		assert.Contains(t, got, "needle")
		assert.Less(t, len(got), 80)
	})
	t.Run("never slices mid-rune", func(t *testing.T) {
		text := strings.Repeat("é", 40) + "needle" + strings.Repeat("Ω", 40)
		got := locatorContext(text, "needle")
		assert.True(t, strings.HasPrefix(got, "…"))
		for _, r := range got {
			assert.NotEqual(t, '�', r, "excerpt must stay valid UTF-8")
		}
	})
}
