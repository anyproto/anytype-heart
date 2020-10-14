package undo

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ base.Base

type groupBlock struct {
	simple.Block
	groupId string
}

func (g *groupBlock) UndoGroupId() string {
	return g.groupId
}

func (g *groupBlock) SetUndoGroupId(id string) {
	g.groupId = id
}

func TestHistory_Add(t *testing.T) {
	t.Run("add with limit", func(t *testing.T) {
		h := NewHistory(2)
		h.Add(Action{Add: []simple.Block{nil}})
		h.Add(Action{Add: []simple.Block{nil}})
		assert.Equal(t, 2, h.Len())
		h.Add(Action{Add: []simple.Block{nil}})
		assert.Equal(t, 2, h.Len())
	})
	t.Run("group", func(t *testing.T) {
		newGroupBlock := func(id, groupId, bg string) simple.Block {
			return &groupBlock{
				Block:   simple.New(&model.Block{Id: id, BackgroundColor: bg}),
				groupId: groupId,
			}
		}

		h := NewHistory(10)
		h.Add(Action{Add: []simple.Block{nil}})
		h.Add(Action{Add: []simple.Block{newGroupBlock("1", "g1", "addFirst")}})
		h.Add(Action{Change: []Change{{After: newGroupBlock("2", "g2", "changeFirst")}}})
		assert.Equal(t, 3, h.Len())

		h.Add(Action{Change: []Change{{After: newGroupBlock("1", "g1", "addSecond")}}})
		assert.Equal(t, 3, h.Len())

		h.Add(Action{Add: []simple.Block{newGroupBlock("2", "g2", "changeSecond")}})
		assert.Equal(t, 3, h.Len())

		h.Add(Action{Change: []Change{{After: newGroupBlock("2", "g2", "changeThird")}}})
		assert.Equal(t, 3, h.Len())

		assert.Equal(t, "addSecond", h.(*history).actions[1].Add[0].Model().BackgroundColor)
		assert.Equal(t, "changeThird", h.(*history).actions[2].Change[0].After.Model().BackgroundColor)
	})
}

func TestHistory_Previous(t *testing.T) {
	t.Run("no undo on empty set", func(t *testing.T) {
		h := NewHistory(3)
		_, err := h.Previous()
		assert.Equal(t, ErrNoHistory, err)
	})
	t.Run("move back", func(t *testing.T) {
		h := NewHistory(2)
		h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "1"})}})
		h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "2"})}})
		h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "3"})}})

		a, err := h.Previous()
		require.NoError(t, err)
		assert.Equal(t, "3", a.Add[0].Model().Id)

		a, err = h.Previous()
		require.NoError(t, err)
		assert.Equal(t, "2", a.Add[0].Model().Id)

		_, err = h.Previous()
		assert.Equal(t, ErrNoHistory, err)
	})
	t.Run("move back and add", func(t *testing.T) {
		h := NewHistory(3)
		h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "1"})}})
		a, err := h.Previous()
		require.NoError(t, err)
		assert.Equal(t, "1", a.Add[0].Model().Id)

		h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "2"})}})
		a, err = h.Previous()
		require.NoError(t, err)
		assert.Equal(t, "2", a.Add[0].Model().Id)

		_, err = h.Previous()
		assert.Equal(t, ErrNoHistory, err)
	})
}

func TestHistory_Next(t *testing.T) {
	t.Run("no undo on empty set", func(t *testing.T) {
		h := NewHistory(3)
		_, err := h.Next()
		assert.Equal(t, ErrNoHistory, err)
	})
	t.Run("move back", func(t *testing.T) {
		h := NewHistory(2)
		h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "1"})}})
		h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "2"})}})
		for i := 0; i < 2; i++ {
			_, err := h.Previous()
			require.NoError(t, err)
		}
		a, err := h.Next()
		require.NoError(t, err)
		assert.Equal(t, "1", a.Add[0].Model().Id)

		a, err = h.Next()
		require.NoError(t, err)
		assert.Equal(t, "2", a.Add[0].Model().Id)

		_, err = h.Next()
		assert.Equal(t, ErrNoHistory, err)
	})
}

func TestHistory_Reset(t *testing.T) {
	h := NewHistory(0)
	h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "1"})}})
	h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "2"})}})
	assert.Equal(t, 2, h.Len())

	h.Reset()
	assert.Equal(t, 0, h.Len())
	_, err := h.Next()
	assert.Equal(t, ErrNoHistory, err)
	_, err = h.Previous()
	assert.Equal(t, ErrNoHistory, err)
}

func TestAction_IsEmpty(t *testing.T) {
	assert.True(t, Action{}.IsEmpty())
	assert.False(t, Action{Add: []simple.Block{nil}}.IsEmpty())
}
