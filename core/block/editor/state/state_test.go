package state

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_Add(t *testing.T) {
	s := NewDoc("1", nil).NewState()
	assert.Nil(t, s.Get("1"))
	assert.True(t, s.Add(base.NewBase(&model.Block{
		Id: "1",
	})))
	assert.NotNil(t, s.Get("1"))
	assert.False(t, s.Add(base.NewBase(&model.Block{
		Id: "1",
	})))
}

func TestState_Get(t *testing.T) {
	s := NewDoc("1", map[string]simple.Block{
		"1": base.NewBase(&model.Block{Id: "1"}),
	}).NewState()
	assert.NotNil(t, s.Get("1"))
	assert.NotNil(t, s.NewState().Get("1"))
}

func TestState_Pick(t *testing.T) {
	s := NewDoc("1", map[string]simple.Block{
		"1": base.NewBase(&model.Block{Id: "1"}),
	}).NewState()
	assert.NotNil(t, s.Pick("1"))
	assert.NotNil(t, s.NewState().Pick("1"))
}

func TestState_Unlink(t *testing.T) {
	s := NewDoc("1", map[string]simple.Block{
		"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
		"2": base.NewBase(&model.Block{Id: "2"}),
	}).NewState()
	assert.True(t, s.Unlink("2"))
	assert.Len(t, s.Pick("1").Model().ChildrenIds, 0)
	assert.False(t, s.Unlink("2"))
}

func TestState_GetParentOf(t *testing.T) {
	t.Run("generic", func(t *testing.T) {
		s := NewDoc("1", map[string]simple.Block{
			"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
			"2": base.NewBase(&model.Block{Id: "2"}),
		}).NewState()
		assert.Equal(t, "1", s.GetParentOf("2").Model().Id)
	})
	t.Run("direct", func(t *testing.T) {
		s := NewDoc("1", map[string]simple.Block{
			"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
			"2": base.NewBase(&model.Block{Id: "2"}),
		}).(*State)
		assert.Equal(t, "1", s.GetParentOf("2").Model().Id)
	})
}

func TestApplyState(t *testing.T) {
	d := NewDoc("1", map[string]simple.Block{
		"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
		"2": base.NewBase(&model.Block{Id: "2"}),
	})
	s := d.NewState()
	s.Add(simple.New(&model.Block{Id: "3"}))
	s.InsertTo("2", model.Block_Bottom, "3")
	s.changeId = "1"

	s = s.NewState()
	s.Add(simple.New(&model.Block{Id: "4"}))
	s.InsertTo("3", model.Block_Bottom, "4")
	s.changeId = "2"

	s = s.NewState()
	s.Unlink("3")
	s.changeId = "3"

	s = s.NewState()
	s.Add(simple.New(&model.Block{Id: "5"}))
	s.InsertTo("4", model.Block_Bottom, "5")
	s.changeId = "4"

	msgs, hist, err := ApplyState(s, true)
	require.NoError(t, err)
	assert.Len(t, hist.Add, 2)
	assert.Len(t, hist.Change, 1)
	assert.Len(t, hist.Remove, 0)
	require.Len(t, msgs, 2)
}

func TestState_Diff(t *testing.T) {
	s1 := NewDoc("root", map[string]simple.Block{
		"root": base.NewBase(&model.Block{Id: "root", ChildrenIds: []string{"2", "3"}}),
		"2":    base.NewBase(&model.Block{Id: "2"}),
		"3":    base.NewBase(&model.Block{Id: "3"}),
	}).NewState()
	s2 := NewDoc("root", map[string]simple.Block{
		"root": base.NewBase(&model.Block{Id: "root", ChildrenIds: []string{"2", "4"}}),
		"2":    base.NewBase(&model.Block{Id: "2"}),
		"4":    base.NewBase(&model.Block{Id: "4"}),
	}).NewState()

	msgs, err := s1.Diff(s2)
	require.NoError(t, err)
	assert.Len(t, msgs, 3)
	assert.NotNil(t, msgs[0].GetBlockSetChildrenIds())
	require.NotNil(t, msgs[1].GetBlockAdd())
	assert.Len(t, msgs[1].GetBlockAdd().Blocks, 1)
	require.NotNil(t, msgs[2].GetBlockDelete())
	assert.Len(t, msgs[2].GetBlockDelete().BlockIds, 1)
}
