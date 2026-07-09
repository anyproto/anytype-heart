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

// newTextBlock is a small helper for building text blocks in tests.
func newTextBlock(id, text string, style model.BlockContentTextStyle, children ...string) simple.Block {
	return simple.New(&model.Block{
		Id:          id,
		ChildrenIds: children,
		Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{Text: text, Style: style},
		},
	})
}

// roundTrip exports the doc to markdown and parses it back to blocks.
func roundTrip(t *testing.T, blocks map[string]simple.Block) (string, []*model.Block) {
	t.Helper()
	s := state.NewDoc("root", blocks).(*state.State)
	c := NewMDConverter(s, nil, false)
	md := string(c.Convert(model.SmartBlockType_Page))
	t.Logf("exported markdown:\n%s", md)
	parsed, _, err := anymark.MarkdownToBlocks([]byte(md), "", nil)
	require.NoError(t, err)
	return md, parsed
}

// B1: a Toggle nested under a Header must round-trip as a Toggle with its child
// still nested. Previously the <details> opener was indented (Header AddSpace =
// 4 spaces), so goldmark parsed it as an indented code block and the whole
// toggle collapsed into one Code block of literal HTML.
func TestMD_ToggleUnderHeaderRoundTrip(t *testing.T) {
	blocks := map[string]simple.Block{
		"root":    simple.New(&model.Block{Id: "root", ChildrenIds: []string{"h1"}}),
		"h1":      newTextBlock("h1", "H", model.BlockContentText_Header1, "toggle1"),
		"toggle1": newTextBlock("toggle1", "T", model.BlockContentText_Toggle, "child1"),
		"child1":  newTextBlock("child1", "Inside", model.BlockContentText_Paragraph),
	}

	_, parsed := roundTrip(t, blocks)

	toggle := findBlockByText(parsed, "T")
	require.NotNil(t, toggle, "expected a toggle block, parsed:\n%+v", parsed)
	assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style,
		"toggle nested under a header must stay a Toggle, parsed:\n%+v", parsed)

	child := findBlockByText(parsed, "Inside")
	require.NotNil(t, child, "expected the child block, parsed:\n%+v", parsed)
	assert.NotEqual(t, model.BlockContentText_Code, child.GetText().Style,
		"child must not collapse into a code block, parsed:\n%+v", parsed)
	assert.Contains(t, toggle.ChildrenIds, child.Id,
		"child must remain nested under the toggle, parsed:\n%+v", parsed)
}

// B2: a Toggle nested under a Checkbox must round-trip as a Toggle with its
// child nested and no leaked indent characters (figure-space U+2007 from
// AddNBSpace). Previously the indent leaked into the child text and the toggle
// style was lost.
func TestMD_ToggleUnderCheckboxRoundTrip(t *testing.T) {
	blocks := map[string]simple.Block{
		"root":    simple.New(&model.Block{Id: "root", ChildrenIds: []string{"chk1"}}),
		"chk1":    newTextBlock("chk1", "Task", model.BlockContentText_Checkbox, "toggle1"),
		"toggle1": newTextBlock("toggle1", "T", model.BlockContentText_Toggle, "child1"),
		"child1":  newTextBlock("child1", "Inside", model.BlockContentText_Paragraph),
	}

	_, parsed := roundTrip(t, blocks)

	toggle := findBlockByText(parsed, "T")
	require.NotNil(t, toggle, "expected a toggle block, parsed:\n%+v", parsed)
	assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style,
		"toggle nested under a checkbox must stay a Toggle, parsed:\n%+v", parsed)

	child := findBlockByText(parsed, "Inside")
	require.NotNil(t, child, "expected the child block (no leaked indent), parsed:\n%+v", parsed)
	assert.Contains(t, toggle.ChildrenIds, child.Id,
		"child must remain nested under the toggle, parsed:\n%+v", parsed)

	// No block text may contain a leaked figure-space (U+2007) indent char.
	for _, b := range parsed {
		if tb := b.GetText(); tb != nil {
			assert.NotContains(t, tb.Text, " ",
				"no leaked figure-space indent expected, block text=%q parsed:\n%+v", tb.Text, parsed)
		}
	}
}

// B3 + M1: summary text containing HTML-significant characters (</details>, <,
// >, &) must round-trip exactly, and the child must stay nested. Previously the
// raw </details> in the summary caused the importer to drop the whole toggle,
// and raw </>/tag-shaped text was stripped.
func TestMD_ToggleSummaryWithSpecialCharsRoundTrip(t *testing.T) {
	const title = "Sneaky </details> a < b && c > d text"
	blocks := map[string]simple.Block{
		"root":    simple.New(&model.Block{Id: "root", ChildrenIds: []string{"toggle1"}}),
		"toggle1": newTextBlock("toggle1", title, model.BlockContentText_Toggle, "child1"),
		"child1":  newTextBlock("child1", "Hidden content", model.BlockContentText_Paragraph),
	}

	_, parsed := roundTrip(t, blocks)

	toggle := findBlockByText(parsed, title)
	require.NotNil(t, toggle,
		"toggle with special-char summary must round-trip its text exactly, parsed:\n%+v", parsed)
	assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style,
		"must stay a Toggle, parsed:\n%+v", parsed)

	child := findBlockByText(parsed, "Hidden content")
	require.NotNil(t, child, "expected the child block, parsed:\n%+v", parsed)
	assert.Contains(t, toggle.ChildrenIds, child.Id,
		"child must remain nested under the toggle, parsed:\n%+v", parsed)
}

// A normal sibling block after a toggle must be unaffected by the toggle's
// column-0 rendering.
func TestMD_SiblingAfterToggleRoundTrip(t *testing.T) {
	blocks := map[string]simple.Block{
		"root":    simple.New(&model.Block{Id: "root", ChildrenIds: []string{"toggle1", "after"}}),
		"toggle1": newTextBlock("toggle1", "My Toggle", model.BlockContentText_Toggle, "child1"),
		"child1":  newTextBlock("child1", "Child content", model.BlockContentText_Paragraph),
		"after":   newTextBlock("after", "After paragraph", model.BlockContentText_Paragraph),
	}

	_, parsed := roundTrip(t, blocks)

	toggle := findBlockByText(parsed, "My Toggle")
	require.NotNil(t, toggle)
	assert.Equal(t, model.BlockContentText_Toggle, toggle.GetText().Style)

	after := findBlockByText(parsed, "After paragraph")
	require.NotNil(t, after, "sibling after toggle must survive, parsed:\n%+v", parsed)
	assert.Equal(t, model.BlockContentText_Paragraph, after.GetText().Style)
	assert.NotContains(t, toggle.ChildrenIds, after.Id,
		"sibling must NOT be nested under the toggle, parsed:\n%+v", parsed)
}

// M2: an empty-summary (untitled) Toggle must round-trip with its child still
// nested. The exporter emits <summary></summary>, and previously the importer's
// parent-merge branch absorbed the first child's text into the empty toggle and
// dropped the nesting.
func TestMD_EmptySummaryToggleRoundTrip(t *testing.T) {
	blocks := map[string]simple.Block{
		"root":    simple.New(&model.Block{Id: "root", ChildrenIds: []string{"toggle1"}}),
		"toggle1": newTextBlock("toggle1", "", model.BlockContentText_Toggle, "child1"),
		"child1":  newTextBlock("child1", "Hidden content", model.BlockContentText_Paragraph),
	}

	md, parsed := roundTrip(t, blocks)
	assert.Contains(t, md, "<summary></summary>", "exporter must emit an empty summary, got:\n%s", md)

	var toggle *model.Block
	for _, b := range parsed {
		if tb := b.GetText(); tb != nil && tb.Style == model.BlockContentText_Toggle {
			toggle = b
			break
		}
	}
	require.NotNil(t, toggle, "expected a toggle block, parsed:\n%+v", parsed)
	assert.Equal(t, "", toggle.GetText().Text,
		"empty-summary toggle must stay empty (no absorbed child text), parsed:\n%+v", parsed)

	child := findBlockByText(parsed, "Hidden content")
	require.NotNil(t, child, "expected the child block, parsed:\n%+v", parsed)
	assert.Contains(t, toggle.ChildrenIds, child.Id,
		"child must remain nested under the empty-summary toggle, parsed:\n%+v", parsed)
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
