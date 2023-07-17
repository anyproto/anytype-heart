package base

import (
	"testing"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBase_Diff(t *testing.T) {
	testBlock := func() *Base {
		return NewBase(&model.Block{
			Fields: &types.Struct{
				Fields: map[string]*types.Value{
					"key": &types.Value{
						Kind: &types.Value_StringValue{StringValue: "value"},
					},
				},
			},
			Restrictions: &model.BlockRestrictions{
				Read:   false,
				Edit:   false,
				Remove: false,
				Drag:   false,
				DropOn: false,
			},
			ChildrenIds: []string{"1", "2", "3"},
		}).(*Base)
	}
	t.Run("equals", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		assert.Len(t, diff, 0)
	})
	t.Run("children ids", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.ChildrenIds[0], b2.ChildrenIds[1] = b2.ChildrenIds[1], b2.ChildrenIds[2]
		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, b2.ChildrenIds, diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetChildrenIds).BlockSetChildrenIds.ChildrenIds)
	})
	t.Run("restrictions", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Restrictions.Read = !b1.Restrictions.Read
		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, b2.Restrictions, diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetRestrictions).BlockSetRestrictions.Restrictions)
	})
	t.Run("fields", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Fields.Fields["diff"] = &types.Value{
			Kind: &types.Value_StringValue{StringValue: "value"},
		}
		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, b2.Fields, diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetFields).BlockSetFields.Fields)
	})
	t.Run("background color", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b1.BackgroundColor = "yellow"
		b2.BackgroundColor = "red"
		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, b2.BackgroundColor, diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetBackgroundColor).BlockSetBackgroundColor.BackgroundColor)
	})
	t.Run("vertical align", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b1.VerticalAlign = model.Block_VerticalAlignTop
		b2.VerticalAlign = model.Block_VerticalAlignMiddle
		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, b2.VerticalAlign, diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetVerticalAlign).BlockSetVerticalAlign.VerticalAlign)
	})
	t.Run("align", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b1.Align = model.Block_AlignLeft
		b2.Align = model.Block_AlignCenter
		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, b2.Align, diff[0].Msg.Value.(*pb.EventMessageValueOfBlockSetAlign).BlockSetAlign.Align)
	})
}
