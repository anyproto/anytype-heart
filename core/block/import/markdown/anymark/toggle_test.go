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
