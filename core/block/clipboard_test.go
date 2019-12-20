package block

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCommonSmart_Paste(t *testing.T) {
	t.Run("should split block on paste", func(t *testing.T) {
		// initial blocks on page
		pageBlocks := []*model.Block{
			{Id: "b1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "11111" }}},
			{Id: "b2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "22222" }}},
			{Id: "b3", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "33333" }}},
			{Id: "b4", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "abcde" }}},
			{Id: "b5", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "55555" }}},
		}
		fx := newPageFixture(t, pageBlocks...)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		err := fx.Paste(pb.RpcBlockPasteRequest{
			FocusedBlockId: "b4",
			SelectedTextRange: &model.Range{From:2, To:4},
			AnySlot: []string{"b2", "b3"},
		})
		require.NoError(t, err)

		// plus 3 blocks in page (4 -> 4a, 4b; 2.1, 3.1)
		require.Len(t, fx.versions[fx.GetId()].Model().ChildrenIds, 8)

		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[0]].Model().GetText().Text, "11111")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[1]].Model().GetText().Text, "22222")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[2]].Model().GetText().Text, "33333")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[3]].Model().GetText().Text, "ab")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[4]].Model().GetText().Text,  "22222")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[5]].Model().GetText().Text,  "33333")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[6]].Model().GetText().Text,  "e")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[7]].Model().GetText().Text, "55555")

		// have 2 events: 1 - show, 2 - update for duplicate
		require.Len(t, fx.serviceFx.events, 2)
		// check we have 3 messages: 4 add + 1 remove children
		assert.Len(t, fx.serviceFx.events[1].Messages, 5)
	})

	t.Run("should paste to the end when no focus", func(t *testing.T) {
		// initial blocks on page
		pageBlocks := []*model.Block{
			{Id: "b1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "11111" }}},
			{Id: "b2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "22222" }}},
			{Id: "b3", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "33333" }}},
			{Id: "b4", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "44444" }}},
			{Id: "b5", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "55555" }}},
		}
		fx := newPageFixture(t, pageBlocks...)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		err := fx.Paste(pb.RpcBlockPasteRequest{
			AnySlot: []string{"b2", "b3"},
		})
		require.NoError(t, err)

		// plus 3 blocks in page (4 -> 4a, 4b; 2.1, 3.1)
		require.Len(t, fx.versions[fx.GetId()].Model().ChildrenIds, 7)

		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[0]].Model().GetText().Text, "11111")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[1]].Model().GetText().Text, "22222")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[2]].Model().GetText().Text, "33333")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[3]].Model().GetText().Text, "44444")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[4]].Model().GetText().Text, "55555")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[5]].Model().GetText().Text, "22222")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[6]].Model().GetText().Text, "33333")

		// have 2 events: 1 - show, 2 - update for duplicate
		require.Len(t, fx.serviceFx.events, 2)
		// check we have 3 messages: 2 add and one change children
		assert.Len(t, fx.serviceFx.events[1].Messages, 3)
	})

	t.Run("should paste after selected blocks if selected == pasted", func(t *testing.T) {
		//initial blocks on page
		pageBlocks := []*model.Block{
			{Id: "b1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "11111" }}},
			{Id: "b2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "22222" }}},
			{Id: "b3", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "33333" }}},
			{Id: "b4", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "44444" }}},
			{Id: "b5", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "55555" }}},
		}
		fx := newPageFixture(t, pageBlocks...)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		err := fx.Paste(pb.RpcBlockPasteRequest{
			SelectedBlockIds: []string{"b2", "b3"},
			AnySlot: []string{"b2", "b3"},
		})
		require.NoError(t, err)

		// plus 3 blocks in page (4 -> 4a, 4b; 2.1, 3.1)
		require.Len(t, fx.versions[fx.GetId()].Model().ChildrenIds, 7)

		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[0]].Model().GetText().Text, "11111")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[1]].Model().GetText().Text, "22222")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[2]].Model().GetText().Text, "33333")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[3]].Model().GetText().Text, "22222")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[4]].Model().GetText().Text, "33333")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[5]].Model().GetText().Text, "44444")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[6]].Model().GetText().Text, "55555")

		// have 2 events: 1 - show, 2 - update for duplicate
		require.Len(t, fx.serviceFx.events, 2)
		// check we have 3 messages: 2 add + 1 set children
		assert.Len(t, fx.serviceFx.events[1].Messages, 3)
	})

	t.Run("should replace selected blocks", func(t *testing.T) {
		// initial blocks on page
		pageBlocks := []*model.Block{
			{Id: "b1", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "11111" }}},
			{Id: "b2", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "22222" }}},
			{Id: "b3", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "33333" }}},
			{Id: "b4", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "44444" }}},
			{Id: "b5", Content: &model.BlockContentOfText{Text: &model.BlockContentText{ Text: "55555" }}},
		}
		fx := newPageFixture(t, pageBlocks...)
		defer fx.ctrl.Finish()
		defer fx.tearDown()

		err := fx.Paste(pb.RpcBlockPasteRequest{
			SelectedBlockIds: []string{"b2", "b3", "b4"},
			AnySlot: []string{"b2", "b3"},
		})
		require.NoError(t, err)

		require.Len(t, fx.versions[fx.GetId()].Model().ChildrenIds, 4)

		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[0]].Model().GetText().Text, "11111")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[1]].Model().GetText().Text, "22222")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[2]].Model().GetText().Text, "33333")
		require.Equal(t, fx.versions[fx.versions[fx.GetId()].Model().ChildrenIds[3]].Model().GetText().Text, "55555")

		// have 2 events: 1 - show, 2 - update for duplicate
		require.Len(t, fx.serviceFx.events, 2)
		// check we have 3 messages: 2 add + 3 remove children + childrenIds
		assert.Len(t, fx.serviceFx.events[1].Messages, 6)
	})
}
