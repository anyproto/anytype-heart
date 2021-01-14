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
		newBlock := func(id, bg string) simple.Block {
			return simple.New(&model.Block{Id: id, BackgroundColor: bg})
		}

		h := NewHistory(10)
		h.Add(Action{Add: []simple.Block{nil}})
		h.Add(Action{Add: []simple.Block{newBlock("1", "addFirst")}, Group: "g1"})
		h.Add(Action{Change: []Change{{After: newBlock("2", "changeFirst")}}, Group: "g2"})
		assert.Equal(t, 3, h.Len())

		h.Add(Action{Change: []Change{{After: newBlock("1", "addSecond")}}, Group: "g1"})
		assert.Equal(t, 3, h.Len())

		h.Add(Action{Change: []Change{{After: newBlock("2", "changeSecond")}}, Group: "g2"})
		assert.Equal(t, 3, h.Len())

		h.Add(Action{Change: []Change{{After: newBlock("2", "changeThird")}, {After: newBlock("3", "")}}, Group: "g2"})
		assert.Equal(t, 3, h.Len())

		assert.Equal(t, "addSecond", h.(*history).actions[1].Add[0].Model().BackgroundColor)
		assert.Equal(t, "changeThird", h.(*history).actions[2].Change[0].After.Model().BackgroundColor)
	})
	t.Run("do not add empty", func(t *testing.T) {
		h := NewHistory(100)
		h.Add(Action{})
		assert.Equal(t, 0, h.Len())
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

func TestHistory_Counters(t *testing.T) {
	h := NewHistory(0)
	h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "1"})}})
	uc, rc := h.Counters()
	assert.Equal(t, int32(1), uc)
	assert.Equal(t, int32(0), rc)
	h.Add(Action{Add: []simple.Block{simple.New(&model.Block{Id: "2"})}})
	uc, rc = h.Counters()
	assert.Equal(t, int32(2), uc)
	assert.Equal(t, int32(0), rc)
	h.Previous()
	uc, rc = h.Counters()
	assert.Equal(t, int32(1), uc)
	assert.Equal(t, int32(1), rc)
	h.Previous()
	uc, rc = h.Counters()
	assert.Equal(t, int32(0), uc)
	assert.Equal(t, int32(2), rc)
	h.Next()
	uc, rc = h.Counters()
	assert.Equal(t, int32(1), uc)
	assert.Equal(t, int32(1), rc)
}
