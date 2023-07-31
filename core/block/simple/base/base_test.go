package base

import (
	"github.com/anyproto/anytype-heart/core/block/simple/test"
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
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetChildrenIds{
			BlockSetChildrenIds: &pb.EventBlockSetChildrenIds{
				Id:          b1.Id,
				ChildrenIds: b2.ChildrenIds,
			},
		}), diff)
	})
	t.Run("restrictions", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Restrictions.Read = !b1.Restrictions.Read
		diff, err := b1.Diff(b2)
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetRestrictions{
			BlockSetRestrictions: &pb.EventBlockSetRestrictions{
				Id: b1.Id,
				Restrictions: &model.BlockRestrictions{
					Read: b2.Restrictions.Read,
				},
			},
		}), diff)
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
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetFields{
			BlockSetFields: &pb.EventBlockSetFields{
				Id: b1.Id,
				Fields: &types.Struct{
					Fields: map[string]*types.Value{
						"diff": {Kind: &types.Value_StringValue{StringValue: "value"}},
						"key":  {Kind: &types.Value_StringValue{StringValue: "value"}},
					},
				},
			},
		}), diff)
	})
	t.Run("changed background color", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.BackgroundColor = "yellow"
		b2.BackgroundColor = "red"
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetBackgroundColor{
			BlockSetBackgroundColor: &pb.EventBlockSetBackgroundColor{
				Id:              b1.Id,
				BackgroundColor: "red",
			},
		}), diff)
	})
	t.Run("changed vertical align", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.VerticalAlign = model.Block_VerticalAlignTop
		b2.VerticalAlign = model.Block_VerticalAlignMiddle
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetVerticalAlign{
			BlockSetVerticalAlign: &pb.EventBlockSetVerticalAlign{
				Id:            b1.Id,
				VerticalAlign: model.Block_VerticalAlignMiddle,
			},
		}), diff)

	})
	t.Run("changed align", func(t *testing.T) {
		// given
		b1 := testBlock()
		b2 := testBlock()

		// when
		b1.Align = model.Block_AlignLeft
		b2.Align = model.Block_AlignCenter
		diff, err := b1.Diff(b2)

		// then
		require.NoError(t, err)
		require.Len(t, diff, 1)
		assert.Equal(t, test.MakeEvent(&pb.EventMessageValueOfBlockSetAlign{
			BlockSetAlign: &pb.EventBlockSetAlign{
				Id:    b1.Id,
				Align: model.Block_AlignCenter,
			},
		}), diff)
	})
}
