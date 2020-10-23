package basic

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasic_Create(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))
		b := NewBasic(sb)
		id, err := b.Create(nil, "", pb.RpcBlockCreateRequest{
			Block: &model.Block{},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id)
		assert.Len(t, sb.Results.Applies, 1)
	})
	t.Run("title", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))
		require.NoError(t, template.ApplyTemplate(sb, sb.NewState(), template.WithTitle))
		b := NewBasic(sb)
		id, err := b.Create(nil, "", pb.RpcBlockCreateRequest{
			TargetId: template.TitleBlockId,
			Position: model.Block_Top,
			Block:    &model.Block{},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id)
		s := sb.NewState()
		assert.Equal(t, []string{template.HeaderLayoutId, id}, s.Pick(s.RootId()).Model().ChildrenIds)
	})
}

func TestBasic_Duplicate(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
		AddBlock(simple.New(&model.Block{Id: "3"}))

	b := NewBasic(sb)
	newIds, err := b.Duplicate(nil, pb.RpcBlockListDuplicateRequest{
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

	err := b.Unlink(nil, "2")
	require.NoError(t, err)
	assert.Nil(t, sb.NewState().Pick("2"))

	assert.Equal(t, smartblock.ErrSimpleBlockNotFound, b.Unlink(nil, "2"))
}

func TestBasic_Move(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2", "4"}})).
			AddBlock(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"3"}})).
			AddBlock(simple.New(&model.Block{Id: "3"})).
			AddBlock(simple.New(&model.Block{Id: "4"}))

		b := NewBasic(sb)

		err := b.Move(nil, pb.RpcBlockListMoveRequest{
			BlockIds:     []string{"3"},
			DropTargetId: "4",
			Position:     model.Block_Inner,
		})
		require.NoError(t, err)
		assert.Len(t, sb.NewState().Pick("2").Model().ChildrenIds, 0)
		assert.Len(t, sb.NewState().Pick("4").Model().ChildrenIds, 1)

	})
	t.Run("header", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test"}))
		require.NoError(t, template.ApplyTemplate(sb, sb.NewState(), template.WithTitle))
		b := NewBasic(sb)
		id1, err := b.Create(nil, "", pb.RpcBlockCreateRequest{
			TargetId: template.HeaderLayoutId,
			Position: model.Block_Bottom,
			Block:    &model.Block{},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id1)
		id0, err := b.Create(nil, "", pb.RpcBlockCreateRequest{
			TargetId: template.HeaderLayoutId,
			Position: model.Block_Bottom,
			Block:    &model.Block{},
		})
		require.NoError(t, err)
		require.NotEmpty(t, id0)

		err = b.Move(nil, pb.RpcBlockListMoveRequest{
			BlockIds:     []string{id0},
			DropTargetId: template.TitleBlockId,
			Position:     model.Block_Top,
		})
		require.NoError(t, err)
		s := sb.NewState()
		assert.Equal(t, []string{template.HeaderLayoutId, id0, id1}, s.Pick(s.RootId()).Model().ChildrenIds)
	})
}

func TestBasic_Replace(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2"}))
	b := NewBasic(sb)
	newId, err := b.Replace(nil, "2", &model.Block{})
	require.NoError(t, err)
	require.NotEmpty(t, newId)
}

func TestBasic_SetFields(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2"}))
	b := NewBasic(sb)

	fields := &types.Struct{
		Fields: map[string]*types.Value{
			"x": pbtypes.String("x"),
		},
	}
	err := b.SetFields(nil, &pb.RpcBlockListSetFieldsRequestBlockField{
		BlockId: "2",
		Fields:  fields,
	})
	require.NoError(t, err)
	assert.Equal(t, fields, sb.NewState().Pick("2").Model().Fields)
}

func TestBasic_Update(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2"}))
	b := NewBasic(sb)

	err := b.Update(nil, func(b simple.Block) error {
		b.Model().BackgroundColor = "test"
		return nil
	}, "2")
	require.NoError(t, err)
	assert.Equal(t, "test", sb.NewState().Pick("2").Model().BackgroundColor)
}

func TestBasic_SetDivStyle(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2", Content: &model.BlockContentOfDiv{Div: &model.BlockContentDiv{}}}))
	b := NewBasic(sb)

	err := b.SetDivStyle(nil, model.BlockContentDiv_Dots, "2")
	require.NoError(t, err)
	r := sb.NewState()
	assert.Equal(t, model.BlockContentDiv_Dots, r.Pick("2").Model().GetDiv().Style)
}

func TestBasic_InternalPaste(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test"}))
	b := NewBasic(sb)
	err := b.InternalPaste([]simple.Block{
		simple.New(&model.Block{Id: "1", ChildrenIds: []string{"1.1"}}),
		simple.New(&model.Block{Id: "1.1", ChildrenIds: []string{"1.1.1"}}),
		simple.New(&model.Block{Id: "1.1.1"}),
		simple.New(&model.Block{Id: "2", ChildrenIds: []string{"2.1"}}),
		simple.New(&model.Block{Id: "2.1"}),
	})
	require.NoError(t, err)
	s := sb.NewState()
	require.Len(t, s.Blocks(), 6)
	assert.Len(t, s.Pick(s.RootId()).Model().ChildrenIds, 2)
}
