package md

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// findBlockByText finds the first block whose text equals the given value.
func findBlockByText(blocks []*model.Block, text string) *model.Block {
	for _, b := range blocks {
		if t := b.GetText(); t != nil && t.Text == text {
			return b
		}
	}
	return nil
}

func TestMD_ToggleExportEncoding(t *testing.T) {
	toggleBlock := &model.Block{
		Id:          "toggle1",
		ChildrenIds: []string{"child1"},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "My Toggle",
				Style: model.BlockContentText_Toggle,
			},
		},
	}
	childBlock := &model.Block{
		Id: "child1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "Child content",
				Style: model.BlockContentText_Paragraph,
			},
		},
	}
	blocks := map[string]simple.Block{
		"root":    simple.New(&model.Block{Id: "root", ChildrenIds: []string{"toggle1"}}),
		"toggle1": simple.New(toggleBlock),
		"child1":  simple.New(childBlock),
	}
	s := state.NewDoc("root", blocks).(*state.State)

	c := NewMDConverter(s, nil, false)
	res := string(c.Convert(model.SmartBlockType_Page))

	// Toggle must NOT be exported as a quote blockquote.
	for _, line := range strings.Split(res, "\n") {
		assert.False(t, strings.HasPrefix(strings.TrimSpace(line), "> My Toggle"),
			"toggle must not be exported as a blockquote, got line: %q in:\n%s", line, res)
	}
	// Toggle is exported as <details>/<summary>.
	assert.Contains(t, res, "<details>", "expected details encoding, got:\n%s", res)
	assert.Contains(t, res, "<summary>My Toggle</summary>", "expected summary with toggle text, got:\n%s", res)
	assert.Contains(t, res, "</details>", "expected details close, got:\n%s", res)
	// Child content is still rendered.
	assert.Contains(t, res, "Child content", "child content must be present, got:\n%s", res)
}

func TestMD_QuoteStillExportsAsBlockquote(t *testing.T) {
	quoteBlock := &model.Block{
		Id: "quote1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "A real quote",
				Style: model.BlockContentText_Quote,
			},
		},
	}
	blocks := map[string]simple.Block{
		"root":   simple.New(&model.Block{Id: "root", ChildrenIds: []string{"quote1"}}),
		"quote1": simple.New(quoteBlock),
	}
	s := state.NewDoc("root", blocks).(*state.State)

	c := NewMDConverter(s, nil, false)
	res := string(c.Convert(model.SmartBlockType_Page))

	assert.Contains(t, res, "> A real quote", "quote must still export as blockquote, got:\n%s", res)
	assert.NotContains(t, res, "<details>", "quote must not be exported as details, got:\n%s", res)
}

// TestMD_ToggleRoundTrip is the core TDD test: build a Toggle+child tree,
// render to markdown, parse it back via anymark, and assert the result is a
// Toggle with the child nested under it.
func TestMD_ToggleRoundTrip(t *testing.T) {
	toggleBlock := &model.Block{
		Id:          "toggle1",
		ChildrenIds: []string{"child1"},
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "My Toggle",
				Style: model.BlockContentText_Toggle,
			},
		},
	}
	childBlock := &model.Block{
		Id: "child1",
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text:  "Child content",
				Style: model.BlockContentText_Paragraph,
			},
		},
	}
	blocks := map[string]simple.Block{
		"root":    simple.New(&model.Block{Id: "root", ChildrenIds: []string{"toggle1"}}),
		"toggle1": simple.New(toggleBlock),
		"child1":  simple.New(childBlock),
	}
	s := state.NewDoc("root", blocks).(*state.State)

	c := NewMDConverter(s, nil, false)
	md := string(c.Convert(model.SmartBlockType_Page))
	t.Logf("exported markdown:\n%s", md)

	parsed, _, err := anymark.MarkdownToBlocks([]byte(md), "", nil)
	require.NoError(t, err)

	toggle := findBlockByText(parsed, "My Toggle")
	require.NotNil(t, toggle, "expected a block with toggle text, parsed:\n%+v", parsed)
	assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style,
		"toggle must round-trip as Toggle style, parsed:\n%+v", parsed)

	child := findBlockByText(parsed, "Child content")
	require.NotNil(t, child, "expected the child block, parsed:\n%+v", parsed)

	// The child must be nested under the toggle, not a top-level sibling.
	assert.Contains(t, toggle.ChildrenIds, child.Id,
		"child must be nested under the toggle, toggle.ChildrenIds=%v child.Id=%s parsed:\n%+v",
		toggle.ChildrenIds, child.Id, parsed)
}
