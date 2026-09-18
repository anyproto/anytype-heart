package clipboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/text"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Every test in this file uses only the public paste API, so it can be run against the
// develop revision of the production files unchanged. TestPasteMarks_DevelopParity and
// TestPasteMarks_ClippingIntoZeroWidthKeepsMark pin behaviour that already worked there and
// must keep passing on both revisions; TestPasteMarks_ClippedMarksKeepTheirSpan asserts the
// mark merge fix and fails on develop by design.

func TestPasteMarks_DevelopParity(t *testing.T) {
	t.Run("mark overrunning the pasted text is clipped and keeps its param", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		pasted := &model.Block{Id: "p", Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "hello",
				Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{{
					Type:  model.BlockContentTextMark_Link,
					Param: "https://example.com/x",
					Range: &model.Range{From: 0, To: 6}, // one past the end of "hello"
				}}},
			},
		}}

		// when
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "1",
			SelectedTextRange: &model.Range{From: 2, To: 2},
			IsPartOfBlock:     true,
			AnySlot:           []*model.Block{pasted},
		}, "")

		// then
		require.NoError(t, err)
		got := sb.Pick("1").Model().GetText()
		assert.Equal(t, "aahelloaa", got.Text)
		require.Len(t, got.Marks.Marks, 1)
		assert.Equal(t, model.BlockContentTextMark_Link, got.Marks.Marks[0].Type)
		assert.Equal(t, "https://example.com/x", got.Marks.Marks[0].Param)
		assert.Equal(t, &model.Range{From: 2, To: 7}, got.Marks.Marks[0].Range)
	})

	t.Run("zero width mark produced by SetMarkForAllText on an empty block survives", func(t *testing.T) {
		// given: build the pasted block the way the editor does, rather than by hand
		src := simple.New(&model.Block{Id: "p", Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{},
		}}).(text.Block)
		src.SetMarkForAllText(&model.BlockContentTextMark{Type: model.BlockContentTextMark_Bold})
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)

		// when
		ids, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			AnySlot: []*model.Block{src.Model()},
		}, "")

		// then
		require.NoError(t, err)
		require.Len(t, ids, 1)
		pastedBlock := simple.New(sb.Pick(ids[0]).Model()).(text.Block)
		assert.True(t, pastedBlock.HasMarkForAllText(&model.BlockContentTextMark{
			Type: model.BlockContentTextMark_Bold,
		}), "bold must still apply to the whole (empty) block, otherwise a later toggle inverts")
	})
}

// Clipping can align two marks of the same type and param on the same start, which makes
// them merge. The merged mark must still span both, not collapse into the shorter one.
func TestPasteMarks_ClippedMarksKeepTheirSpan(t *testing.T) {
	t.Run("two link marks clipped to the same start keep the full span and param", func(t *testing.T) {
		// given
		sb := createPage(t, createBlocks([]string{}, []string{"aa"}, emptyMarks))
		cb := newFixture(t, sb)
		linkMark := func(from, to int32) *model.BlockContentTextMark {
			return &model.BlockContentTextMark{
				Type:  model.BlockContentTextMark_Link,
				Param: "https://example.com/x",
				Range: &model.Range{From: from, To: to},
			}
		}

		// when: the first mark covers all of "hello", the second only its start
		_, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{
			FocusedBlockId:    "1",
			SelectedTextRange: &model.Range{From: 1, To: 1},
			IsPartOfBlock:     true,
			AnySlot: []*model.Block{{Id: "p", Content: &model.BlockContentOfText{
				Text: &model.BlockContentText{
					Text: "hello",
					Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{
						linkMark(0, 5), linkMark(-3, 2),
					}},
				},
			}}},
		}, "")

		// then
		require.NoError(t, err)
		got := sb.Pick("1").Model().GetText()
		assert.Equal(t, "ahelloa", got.Text)
		require.Len(t, got.Marks.Marks, 1)
		assert.Equal(t, "https://example.com/x", got.Marks.Marks[0].Param)
		assert.Equal(t, &model.Range{From: 1, To: 6}, got.Marks.Marks[0].Range,
			"the link must still cover all of the pasted word, not collapse into the shorter mark")
	})
}

// Clipping the upper bound can legitimately produce a zero-width range. That is the same
// shape SetMarkForAllText gives an empty block, so the repaired mark must be kept.
func TestPasteMarks_ClippingIntoZeroWidthKeepsMark(t *testing.T) {
	t.Run("bold clipped onto empty text still marks the whole block", func(t *testing.T) {
		// given: an empty paragraph carrying bold [0,1], which clips to [0,0]
		sb := createPage(t, createBlocks([]string{}, []string{"aaaa"}, emptyMarks))
		cb := newFixture(t, sb)
		pasted := &model.Block{Id: "p", Content: &model.BlockContentOfText{
			Text: &model.BlockContentText{
				Text: "",
				Marks: &model.BlockContentTextMarks{Marks: []*model.BlockContentTextMark{{
					Type:  model.BlockContentTextMark_Bold,
					Range: &model.Range{From: 0, To: 1},
				}}},
			},
		}}

		// when
		ids, _, _, _, err := cb.Paste(nil, &pb.RpcBlockPasteRequest{AnySlot: []*model.Block{pasted}}, "")

		// then
		require.NoError(t, err)
		require.Len(t, ids, 1)
		pastedBlock := simple.New(sb.Pick(ids[0]).Model()).(text.Block)
		assert.True(t, pastedBlock.HasMarkForAllText(&model.BlockContentTextMark{
			Type: model.BlockContentTextMark_Bold,
		}), "whole block bold must survive a range clipped down to zero width")
	})
}
