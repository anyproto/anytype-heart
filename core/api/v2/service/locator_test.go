package v2service

// locator_test.go — unit pins for resolveByFind (Wave 2.1a, §8.43) that
// the service-level tests in edit_test.go cannot reach: the candidate cap
// (documents with >8 matching blocks) and the text-bearing gate (the real
// format never puts `text` on a non-text block, so the gate is
// unobservable through PatchObject — it is the invariant that a block
// replaceText cannot edit never captures or contaminates a match).
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

func TestResolveByFind(t *testing.T) {
	t.Run("the candidate list caps at 8 and counts the rest", func(t *testing.T) {
		blocks := make([]string, 10)
		for i := range blocks {
			blocks[i] = fmt.Sprintf(`{"id":"blk%02d","type":"paragraph","text":"needle %d"}`, i, i)
		}
		doc := findDoc(t, blocks...)

		_, err := resolveByFind(doc, "needle", "ops[0].find")

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeAmbiguousInput, apiErr.Code)
		assert.Contains(t, apiErr.Message, `"needle" appears in 10 blocks`)
		assert.Contains(t, apiErr.Message, "block blk07 (paragraph)", "the 8th candidate is listed")
		assert.NotContains(t, apiErr.Message, "blk08", "the 9th is not")
		assert.Contains(t, apiErr.Message, "… and 2 more")
	})

	t.Run("a non-text block never captures the match", func(t *testing.T) {
		// were a non-text block to carry the snippet, resolving to it would
		// produce an op the applier must then refuse — so it must not count
		doc := findDoc(t,
			`{"id":"bmOne1","type":"bookmark","text":"needle"}`,
			`{"id":"parOne1","type":"paragraph","text":"needle"}`)

		idx, err := resolveByFind(doc, "needle", "ops[0].find")

		require.NoError(t, err, "the paragraph is the unique TEXT match — no ambiguity")
		assert.Equal(t, 1, idx)
	})

	t.Run("a snippet found only in a non-text block is zero matches", func(t *testing.T) {
		doc := findDoc(t,
			`{"id":"bmOne1","type":"bookmark","text":"needle"}`,
			`{"id":"parOne1","type":"paragraph","text":"other"}`)

		_, err := resolveByFind(doc, "needle", "ops[0].find")

		apiErr := v2Err(t, err)
		assert.Equal(t, v2model.CodeNotFound, apiErr.Code)
	})

	t.Run("code and embed text participates (§8.4 literal channels)", func(t *testing.T) {
		doc := findDoc(t,
			`{"id":"parOne1","type":"paragraph","text":"prose"}`,
			`{"id":"codeOne1","type":"code","text":"var needle int"}`)

		idx, err := resolveByFind(doc, "needle", "ops[0].find")

		require.NoError(t, err)
		assert.Equal(t, 1, idx)
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
