package widget

import (
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDiff(t *testing.T) {
	testBlock := func() *block {
		return NewBlock(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfWidget{Widget: &model.BlockContentWidget{}},
		}).(*block)
	}
	t.Run("layout", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.content.Layout = model.BlockContentWidget_Tree

		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetWidget).BlockSetWidget.Layout
		changeLimit := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetWidget).BlockSetWidget.Limit
		changeViewID := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetWidget).BlockSetWidget.ViewId
		assert.Equal(t, model.BlockContentWidget_Tree, change.Value)
		assert.Nil(t, changeLimit)
		assert.Nil(t, changeViewID)
	})
	t.Run("view id", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.content.ViewId = "viewID"

		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetWidget).BlockSetWidget.ViewId
		changeLimit := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetWidget).BlockSetWidget.Limit
		changeLayout := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetWidget).BlockSetWidget.Layout
		assert.Equal(t, "viewID", change.Value)
		assert.Nil(t, changeLimit)
		assert.Nil(t, changeLayout)
	})
	t.Run("limit", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.content.Limit = 10

		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		change := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetWidget).BlockSetWidget.Limit
		changeLayout := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetWidget).BlockSetWidget.Layout
		changeViewID := diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetWidget).BlockSetWidget.ViewId
		assert.Equal(t, int32(10), change.Value)
		assert.Nil(t, changeLayout)
		assert.Nil(t, changeViewID)
	})
}
