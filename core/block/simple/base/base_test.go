package base

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
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
			Permissions: &model.BlockPermissions{
				Read:   true,
				Edit:   true,
				Remove: true,
				Drag:   true,
				DropOn: true,
			},
			ChildrenIds: []string{"1", "2", "3"},
			IsArchived:  false,
		})
	}
	t.Run("equals", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		assert.Len(t, b1.Diff(b2.Model()), 0)
	})
	t.Run("children ids", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.ChildrenIds[0], b2.ChildrenIds[1] = b2.ChildrenIds[1], b2.ChildrenIds[2]
		diff := b1.Diff(b2.Model())
		require.Len(t, diff, 1)
		assert.Equal(t, b2.ChildrenIds, diff[0].Value.(*pb.EventMessageValueOfBlockSetChildrenIds).BlockSetChildrenIds.ChildrenIds)
	})
	t.Run("isArchived", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.IsArchived = !b1.IsArchived
		diff := b1.Diff(b2.Model())
		require.Len(t, diff, 1)
		assert.Equal(t, b2.IsArchived, diff[0].Value.(*pb.EventMessageValueOfBlockSetIsArchived).BlockSetIsArchived.IsArchived)
	})
	t.Run("permissions", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Permissions.Read = !b1.Permissions.Read
		diff := b1.Diff(b2.Model())
		require.Len(t, diff, 1)
		assert.Equal(t, b2.Permissions, diff[0].Value.(*pb.EventMessageValueOfBlockSetPermissions).BlockSetPermissions.Permissions)
	})
	t.Run("fields", func(t *testing.T) {
		b1 := testBlock()
		b2 := testBlock()
		b2.Fields.Fields["diff"] = &types.Value{
			Kind: &types.Value_StringValue{StringValue: "value"},
		}
		diff := b1.Diff(b2.Model())
		require.Len(t, diff, 1)
		assert.Equal(t, b2.Fields, diff[0].Value.(*pb.EventMessageValueOfBlockSetFields).BlockSetFields.Fields)
	})
}
