package basic

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHistory_Undo(t *testing.T) {
	sb := smarttest.New("test")
	sb.AddBlock(simple.New(&model.Block{Id: "test", ChildrenIds: []string{"2"}})).
		AddBlock(simple.New(&model.Block{Id: "2"}))

	s := sb.NewState()
	s.Unlink("2")
	require.NoError(t, sb.Apply(s))
	require.Len(t, sb.Blocks(), 1)

	h := NewHistory(sb)

	err := h.Undo(nil)
	require.NoError(t, err)
	assert.Len(t, sb.Blocks(), 2)
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

	require.NoError(t, h.Undo(nil))

	err := h.Redo(nil)
	require.NoError(t, err)
	assert.Len(t, sb.Blocks(), 1)
}
