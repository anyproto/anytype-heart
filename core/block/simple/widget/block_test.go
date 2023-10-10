package widget

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/simple/test"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestDiff(t *testing.T) {
	testBlock := func() *block {
		return NewBlock(&model.Block{
			Restrictions: &model.BlockRestrictions{},
			Content:      &model.BlockContentOfWidget{Widget: &model.BlockContentWidget{}},
		}).(*block)
	}
	t.Run("change widget layout", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b2.content.Layout = model.BlockContentWidget_Tree
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetWidget{
			BlockSetWidget: &pb.EventBlockSetWidget{
				Id:     b1.Id,
				Layout: &pb.EventBlockSetWidgetLayout{Value: model.BlockContentWidget_Tree},
				Limit:  nil,
				ViewId: nil,
			},
		}), diff)
	})
	t.Run("view id changed", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b2.content.ViewId = "viewID"
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)

		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetWidget{
			BlockSetWidget: &pb.EventBlockSetWidget{
				Id:     b1.Id,
				Layout: nil,
				Limit:  nil,
				ViewId: &pb.EventBlockSetWidgetViewId{Value: "viewID"},
			},
		}), diff)
	})
	t.Run("limit changed", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b2.content.Limit = 10
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetWidget{
			BlockSetWidget: &pb.EventBlockSetWidget{
				Id:     b1.Id,
				Layout: nil,
				Limit:  &pb.EventBlockSetWidgetLimit{Value: 10},
				ViewId: nil,
			},
		}), diff)
	})
}
