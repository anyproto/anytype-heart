package anymark

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func blockByText(blocks []*model.Block, text string) *model.Block {
	for _, b := range blocks {
		if t := b.GetText(); t != nil && t.Text == text {
			return b
		}
	}
	return nil
}

func TestMarkdownToBlocks_DetailsToggle(t *testing.T) {
	t.Run("details with single child parses to nested toggle", func(t *testing.T) {
		md := "<details>\n<summary>My Toggle</summary>\n\nChild content\n\n</details>\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		toggle := blockByText(blocks, "My Toggle")
		require.NotNil(t, toggle, "expected a toggle block, got: %+v", blocks)
		assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style)

		child := blockByText(blocks, "Child content")
		require.NotNil(t, child, "expected a child block, got: %+v", blocks)
		assert.NotEqual(t, model.BlockContentText_Code, child.GetText().Style,
			"child must not be parsed as a code block")
		assert.Contains(t, toggle.ChildrenIds, child.Id,
			"child must be nested under the toggle, got: %+v", blocks)
	})

	t.Run("details with multiple children parses to nested toggle", func(t *testing.T) {
		md := "<details>\n<summary>Group</summary>\n\nFirst\n\nSecond\n\n</details>\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		toggle := blockByText(blocks, "Group")
		require.NotNil(t, toggle)
		assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style)

		first := blockByText(blocks, "First")
		second := blockByText(blocks, "Second")
		require.NotNil(t, first)
		require.NotNil(t, second)
		assert.Contains(t, toggle.ChildrenIds, first.Id)
		assert.Contains(t, toggle.ChildrenIds, second.Id)
	})

	t.Run("escaped summary entities decode to original text", func(t *testing.T) {
		// The exporter HTML-escapes the summary, so </details>, <, >, & arrive
		// as entities. The importer must decode them back and keep the child
		// nested (the opener line must no longer contain a literal </details>).
		md := "<details>\n<summary>Sneaky &lt;/details&gt; a &lt; b &amp;&amp; c &gt; d text</summary>\n\nHidden content\n\n</details>\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		const want = "Sneaky </details> a < b && c > d text"
		toggle := blockByText(blocks, want)
		require.NotNil(t, toggle, "expected toggle with decoded summary, got: %+v", blocks)
		assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style)

		child := blockByText(blocks, "Hidden content")
		require.NotNil(t, child, "expected child block, got: %+v", blocks)
		assert.Contains(t, toggle.ChildrenIds, child.Id,
			"child must be nested under the toggle, got: %+v", blocks)
	})

	t.Run("plain blockquote still parses to Quote", func(t *testing.T) {
		md := "> A real quote\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		quote := blockByText(blocks, "A real quote")
		require.NotNil(t, quote, "expected a quote block, got: %+v", blocks)
		assert.Equal(t, model.BlockContentText_Quote, quote.GetText().Style)
	})
}

// M2: an empty-summary <details> must still nest its children. Previously the
// toggle's empty Text caused the parent-merge branch in CloseTextBlock to absorb
// the FIRST child's text into the toggle and drop it as a child.
func TestMarkdownToBlocks_EmptySummaryToggle(t *testing.T) {
	t.Run("empty summary with single child nests the child", func(t *testing.T) {
		md := "<details>\n<summary></summary>\n\nHidden content\n\n</details>\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		var toggle *model.Block
		for _, b := range blocks {
			if t := b.GetText(); t != nil && t.Style == model.BlockContentText_Toggle {
				toggle = b
				break
			}
		}
		require.NotNil(t, toggle, "expected a toggle block, got: %+v", blocks)
		assert.Equal(t, "", toggle.GetText().Text,
			"empty-summary toggle must have empty text (no absorbed child text), got: %+v", blocks)

		child := blockByText(blocks, "Hidden content")
		require.NotNil(t, child, "expected child block 'Hidden content', got: %+v", blocks)
		assert.Contains(t, toggle.ChildrenIds, child.Id,
			"child must be nested under the empty-summary toggle, got: %+v", blocks)
	})

	t.Run("empty summary with multiple children nests all", func(t *testing.T) {
		md := "<details>\n<summary></summary>\n\nFirst\n\nSecond\n\n</details>\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		var toggle *model.Block
		for _, b := range blocks {
			if t := b.GetText(); t != nil && t.Style == model.BlockContentText_Toggle {
				toggle = b
				break
			}
		}
		require.NotNil(t, toggle, "expected a toggle block, got: %+v", blocks)
		assert.Equal(t, "", toggle.GetText().Text,
			"empty-summary toggle must have empty text, got: %+v", blocks)

		first := blockByText(blocks, "First")
		second := blockByText(blocks, "Second")
		require.NotNil(t, first, "expected 'First', got: %+v", blocks)
		require.NotNil(t, second, "expected 'Second', got: %+v", blocks)
		assert.Contains(t, toggle.ChildrenIds, first.Id,
			"first child must be nested, got: %+v", blocks)
		assert.Contains(t, toggle.ChildrenIds, second.Id,
			"second child must be nested, got: %+v", blocks)
	})
}

// M3: a self-contained <details>...</details> with no blank line before the
// children is emitted by goldmark as a SINGLE HTMLBlock. Previously
// parseDetailsSummary returned false for the "both open and close present" case,
// so the whole block (summary + children) was silently dropped.
func TestMarkdownToBlocks_TightDetailsToggle(t *testing.T) {
	t.Run("tight details with single child nests the child", func(t *testing.T) {
		md := "<details>\n<summary>S</summary>\nChild\n</details>\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		toggle := blockByText(blocks, "S")
		require.NotNil(t, toggle, "expected a toggle block with summary 'S', got: %+v", blocks)
		assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style)

		child := blockByText(blocks, "Child")
		require.NotNil(t, child, "expected child block 'Child', got: %+v", blocks)
		assert.Contains(t, toggle.ChildrenIds, child.Id,
			"child must be nested under the tight toggle, got: %+v", blocks)
	})

	t.Run("tight details with multiple children nests all", func(t *testing.T) {
		md := "<details>\n<summary>Group</summary>\nFirst\n\nSecond\n</details>\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		toggle := blockByText(blocks, "Group")
		require.NotNil(t, toggle, "expected a toggle block, got: %+v", blocks)
		assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style)

		first := blockByText(blocks, "First")
		second := blockByText(blocks, "Second")
		require.NotNil(t, first, "expected 'First', got: %+v", blocks)
		require.NotNil(t, second, "expected 'Second', got: %+v", blocks)
		assert.Contains(t, toggle.ChildrenIds, first.Id)
		assert.Contains(t, toggle.ChildrenIds, second.Id)
	})

	t.Run("tight details summary entities are decoded", func(t *testing.T) {
		md := "<details>\n<summary>A &lt; B &amp; C</summary>\nChild\n</details>\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		toggle := blockByText(blocks, "A < B & C")
		require.NotNil(t, toggle, "expected toggle with decoded summary, got: %+v", blocks)
		assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style)

		child := blockByText(blocks, "Child")
		require.NotNil(t, child, "expected child block, got: %+v", blocks)
		assert.Contains(t, toggle.ChildrenIds, child.Id)
	})

	t.Run("empty self-contained details produces a childless toggle (no crash)", func(t *testing.T) {
		md := "<details>\n<summary>Empty</summary>\n</details>\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		toggle := blockByText(blocks, "Empty")
		require.NotNil(t, toggle, "expected a toggle block, got: %+v", blocks)
		assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style)
		assert.Empty(t, toggle.ChildrenIds, "empty toggle must have no children, got: %+v", blocks)
	})

	t.Run("nested tight details nests inner toggle under outer", func(t *testing.T) {
		md := "<details>\n<summary>Outer</summary>\n<details>\n<summary>Inner</summary>\nDeep\n</details>\n</details>\n"

		blocks, _, err := MarkdownToBlocks([]byte(md), "", nil)
		require.NoError(t, err)

		outer := blockByText(blocks, "Outer")
		require.NotNil(t, outer, "expected outer toggle, got: %+v", blocks)
		assert.Equal(t, model.BlockContentText_Toggle, outer.GetText().Style)

		inner := blockByText(blocks, "Inner")
		require.NotNil(t, inner, "expected inner toggle, got: %+v", blocks)
		assert.Equal(t, model.BlockContentText_Toggle, inner.GetText().Style)
		assert.Contains(t, outer.ChildrenIds, inner.Id,
			"inner toggle must be nested under outer, got: %+v", blocks)

		deep := blockByText(blocks, "Deep")
		require.NotNil(t, deep, "expected deep child, got: %+v", blocks)
		assert.Contains(t, inner.ChildrenIds, deep.Id,
			"deep child must be nested under inner toggle, got: %+v", blocks)
	})
}

func TestHTMLToBlocks_DetailsToggle(t *testing.T) {
	t.Run("html details parses to nested toggle", func(t *testing.T) {
		html := "<details><summary>My Toggle</summary><p>Child content</p></details>"

		blocks, _, err := HTMLToBlocks([]byte(html), "")
		require.NoError(t, err)

		toggle := blockByText(blocks, "My Toggle")
		require.NotNil(t, toggle, "expected a toggle block, got: %+v", blocks)
		assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style)

		child := blockByText(blocks, "Child content")
		require.NotNil(t, child, "expected a child block, got: %+v", blocks)
		assert.Contains(t, toggle.ChildrenIds, child.Id)
	})
}
