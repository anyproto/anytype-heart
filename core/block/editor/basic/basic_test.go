package basic

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasic_Create(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test"}))
	b := NewBasic(sb)
	id, err := b.Create(pb.RpcBlockCreateRequest{
		Block: &model.Block{},
	})
	require.NoError(t, err)
	require.NotEmpty(t, id)
	assert.Len(t, sb.Results.Applies, 1)
}

func TestBasic_Duplicate(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
		AddBlock(simple.New(&model.Block{Id: "3"}))

	b := NewBasic(sb)
	newIds, err := b.Duplicate(pb.RpcBlockListDuplicateRequest{
		BlockIds: []string{"2"},
	})
	require.NoError(t, err)
	require.Len(t, newIds, 1)
	s := sb.NewState()
	assert.Len(t, s.Pick(newIds[0]).Model().ChildrenIds, 1)
	assert.Len(t, sb.Blocks(), 5)
}

func TestBasic_Unlink(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
		AddBlock(simple.New(&model.Block{Id: "3"}))

	b := NewBasic(sb)

	err := b.Unlink("2")
	require.NoError(t, err)
	assert.Nil(t, sb.NewState().Pick("2"))

	assert.Equal(t, smartblock.ErrSimpleBlockNotFound, b.Unlink("2"))
}

func TestBasic_Move(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2", "4"}})).
		AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
		AddBlock(simple.New(&model.Block{Id: "3"})).
		AddBlock(simple.New(&model.Block{Id: "4"}))

	b := NewBasic(sb)

	err := b.Move(pb.RpcBlockListMoveRequest{
		BlockIds:     []string{"3"},
		DropTargetId: "4",
		Position:     model.Block_Inner,
	})
	require.NoError(t, err)
	assert.Len(t, sb.NewState().Pick("2").Model().ChildrenIds, 0)
	assert.Len(t, sb.NewState().Pick("4").Model().ChildrenIds, 1)
}
