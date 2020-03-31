package state

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/stretchr/testify/assert"
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

func TestState_Remove(t *testing.T) {
	s := NewDoc("1", map[string]simple.Block{
		"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
		"2": base.NewBase(&model.Block{Id: "2"}),
	}).NewState()
	assert.True(t, s.Remove("2"))
	assert.Len(t, s.Pick("1").Model().ChildrenIds, 0)
	assert.False(t, s.Remove("2"))
	assert.Nil(t, s.Get("2"))
	assert.Nil(t, s.Pick("2"))
}

func TestState_GetParentOf(t *testing.T) {
	s := NewDoc("1", map[string]simple.Block{
		"1": base.NewBase(&model.Block{Id: "1", ChildrenIds: []string{"2"}}),
		"2": base.NewBase(&model.Block{Id: "2"}),
	}).NewState()
	assert.Equal(t, "1", s.GetParentOf("2").Model().Id)
}
