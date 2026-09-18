package md

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// quoteStateWithMarks builds a single-block doc of the given style carrying the
// given marks.
func quoteStateWithMarks(text string, style model.BlockContentTextStyle, marks ...*model.BlockContentTextMark) *state.State {
	blocks := map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"b1"}}),
		"b1": simple.New(&model.Block{
			Id: "b1",
			Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text:  text,
					Style: style,
					Marks: &model.BlockContentTextMarks{Marks: marks},
				},
			},
		}),
	}
	return state.NewDoc("root", blocks).(*state.State)
}

func mark(from, to int32, markType model.BlockContentTextMarkType, param string) *model.BlockContentTextMark {
	return &model.BlockContentTextMark{
		Range: &model.Range{From: from, To: to},
		Type:  markType,
		Param: param,
	}
}

// GO-7514: a Quote block used to be exported by writing text.Text straight into
// the buffer, bypassing the mark writer, so every mark (link, bold, italic,
// code, ...) was silently dropped on markdown export.
func TestMD_QuoteExportsMarks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		text  string
		marks []*model.BlockContentTextMark
		want  string
	}{
		{
			name:  "quote with link mark keeps the destination",
			text:  "Alpha",
			marks: []*model.BlockContentTextMark{mark(0, 5, model.BlockContentTextMark_Link, "https://example.com/target")},
			want:  "> [Alpha](https://example.com/target)   \n\n",
		},
		{
			name:  "quote with bold mark",
			text:  "Alpha",
			marks: []*model.BlockContentTextMark{mark(0, 5, model.BlockContentTextMark_Bold, "")},
			want:  "> **Alpha**   \n\n",
		},
		{
			name:  "quote with italic mark",
			text:  "Alpha",
			marks: []*model.BlockContentTextMark{mark(0, 5, model.BlockContentTextMark_Italic, "")},
			want:  "> *Alpha*   \n\n",
		},
		{
			name:  "quote with keyboard (code) mark",
			text:  "Alpha",
			marks: []*model.BlockContentTextMark{mark(0, 5, model.BlockContentTextMark_Keyboard, "")},
			want:  "> `Alpha`   \n\n",
		},
		{
			name:  "quote with strikethrough mark",
			text:  "Alpha",
			marks: []*model.BlockContentTextMark{mark(0, 5, model.BlockContentTextMark_Strikethrough, "")},
			want:  "> ~~Alpha~~   \n\n",
		},
		{
			name:  "quote with partial mark keeps the unmarked remainder",
			text:  "Alpha Beta",
			marks: []*model.BlockContentTextMark{mark(0, 5, model.BlockContentTextMark_Link, "https://example.com/target")},
			want:  "> [Alpha](https://example.com/target) Beta   \n\n",
		},
		{
			name:  "quote without marks is unchanged",
			text:  "Alpha",
			marks: nil,
			want:  "> Alpha   \n\n",
		},
		{
			name:  "multi-line quote without marks keeps the per-line prefix",
			text:  "Alpha\nBeta",
			marks: nil,
			want:  "> Alpha   \n> Beta   \n\n",
		},
		{
			name:  "multi-line quote keeps the per-line prefix with marks",
			text:  "Alpha\nBeta",
			marks: []*model.BlockContentTextMark{mark(6, 10, model.BlockContentTextMark_Link, "https://example.com/target")},
			want:  "> Alpha   \n> [Beta](https://example.com/target)   \n\n",
		},
		{
			name:  "empty quote",
			text:  "",
			marks: nil,
			want:  ">    \n\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			s := quoteStateWithMarks(tc.text, model.BlockContentText_Quote, tc.marks...)

			// when
			got := string(NewMDConverter(s, &testFileNamer{}, false).Convert(model.SmartBlockType_Page))

			// then
			assert.Equal(t, tc.want, got)
		})
	}
}

// A Quote must render its marks exactly the way a Paragraph with the identical
// marks does — the only difference being the blockquote prefix.
func TestMD_QuoteMarksMatchParagraphMarks(t *testing.T) {
	// given
	marks := []*model.BlockContentTextMark{
		mark(0, 5, model.BlockContentTextMark_Link, "https://example.com/target"),
		mark(6, 10, model.BlockContentTextMark_Bold, ""),
	}
	want := "[Alpha](https://example.com/target) **Beta**"

	// when
	quote := string(NewMDConverter(quoteStateWithMarks("Alpha Beta", model.BlockContentText_Quote, marks...), &testFileNamer{}, false).Convert(model.SmartBlockType_Page))
	paragraph := string(NewMDConverter(quoteStateWithMarks("Alpha Beta", model.BlockContentText_Paragraph, marks...), &testFileNamer{}, false).Convert(model.SmartBlockType_Page))

	// then
	assert.Equal(t, "> "+want+"   \n\n", quote)
	assert.Equal(t, want+"   \n", paragraph)
}

// The quote's children must still be rendered after the quote line.
func TestMD_QuoteWithMarksStillRendersChildren(t *testing.T) {
	// given
	blocks := map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"quote1"}}),
		"quote1": simple.New(&model.Block{Id: "quote1", ChildrenIds: []string{"child1"}, Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  "Alpha",
			Style: model.BlockContentText_Quote,
			Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				mark(0, 5, model.BlockContentTextMark_Link, "https://example.com/target"),
			}},
		}}}),
		"child1": newTextBlock("child1", "Child content", model.BlockContentText_Paragraph),
	}
	s := state.NewDoc("root", blocks).(*state.State)
	want := "> [Alpha](https://example.com/target)   \n\nChild content   \n"

	// when
	got := string(NewMDConverter(s, &testFileNamer{}, false).Convert(model.SmartBlockType_Page))

	// then
	assert.Equal(t, want, got)
}

// A quote carrying a link must survive an export -> import round trip with both
// its Quote style and its link mark intact.
func TestMD_QuoteWithLinkRoundTrip(t *testing.T) {
	// given
	blocks := map[string]simple.Block{
		"root": simple.New(&model.Block{Id: "root", ChildrenIds: []string{"quote1"}}),
		"quote1": simple.New(&model.Block{Id: "quote1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
			Text:  "Alpha",
			Style: model.BlockContentText_Quote,
			Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
				mark(0, 5, model.BlockContentTextMark_Link, "https://example.com/target"),
			}},
		}}}),
	}

	// when
	md, parsed := roundTrip(t, blocks)

	// then
	assert.Contains(t, md, "[Alpha](https://example.com/target)")
	quote := findBlockByText(parsed, "Alpha")
	require.NotNil(t, quote, "expected the quote block back, parsed:\n%+v", parsed)
	assert.Equal(t, model.BlockContentText_Quote, quote.GetText().Style)
	require.NotNil(t, quote.GetText().Marks, "expected marks on the imported quote, parsed:\n%+v", parsed)
	require.Len(t, quote.GetText().Marks.Marks, 1, "expected exactly one mark, parsed:\n%+v", parsed)
	assert.Equal(t, model.BlockContentTextMark_Link, quote.GetText().Marks.Marks[0].Type)
	assert.Equal(t, "https://example.com/target", quote.GetText().Marks.Marks[0].Param)
}
