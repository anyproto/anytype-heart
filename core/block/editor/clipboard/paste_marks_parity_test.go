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

// These tests pin behaviour that already worked on develop. They use only the public paste
// API so they can be run against the develop revision of the production files unchanged.

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
