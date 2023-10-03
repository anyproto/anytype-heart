package basic

import (
	"strings"
	"testing"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistory_Undo(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		bDetails := &types.Struct{
			Fields: map[string]*types.Value{
				"beforeK": pbtypes.String("beforeV"),
			},
		}
		aDetails := &types.Struct{
			Fields: map[string]*types.Value{
				"afterK": pbtypes.String("afterV"),
			},
		}
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
			AddBlock(simple.New(&model.Block{Id: "2"}))
		sb.Doc.(*state.State).SetDetails(bDetails)

		s := sb.NewState()
		s.Unlink("2")
		s.SetDetails(aDetails)
		require.NoError(t, sb.Apply(s))
		require.Len(t, sb.Blocks(), 1)
		assert.True(t, aDetails.Equal(sb.Details()))

		h := NewHistory(sb)

		_, err := h.Undo(nil)
		require.NoError(t, err)
		assert.Len(t, sb.Blocks(), 2)
		assert.Equal(t, bDetails, sb.Details())
	})
	t.Run("column remove undo", func(t *testing.T) {
		sb := smarttest.New("test")
		sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2", "3"}})).
			AddBlock(simple.New(&model.Block{Id: "2"})).
			AddBlock(simple.New(&model.Block{Id: "3"}))
		s := sb.NewState()
		s.Unlink("3")
		require.NoError(t, s.InsertTo("2", model.Block_Right, "3"))
		require.NoError(t, sb.Apply(s))
		//t.Log(sb.Doc.(*state.State).String())

		s = sb.NewState()
		s.Unlink("3")
		require.NoError(t, sb.Apply(s))
		require.Len(t, sb.Doc.Pick("test").Model().ChildrenIds, 1)

		h := NewHistory(sb)

		_, err := h.Undo(nil)
		require.NoError(t, err)
		//t.Log(sb.Doc.(*state.State).String())
		require.Len(t, sb.Doc.Pick("test").Model().ChildrenIds, 1)
		assert.True(t, strings.HasPrefix(sb.Doc.Pick("test").Model().ChildrenIds[0], "r-"))
	})
}

func TestHistory_Redo(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2"}))

	s := sb.NewState()
	s.Unlink("2")
	require.NoError(t, sb.Apply(s))
	require.Len(t, sb.Blocks(), 1)

	h := NewHistory(sb)
	_, err := h.Undo(nil)
	require.NoError(t, err)

	_, err = h.Redo(nil)
	require.NoError(t, err)
	assert.Len(t, sb.Blocks(), 1)
}
