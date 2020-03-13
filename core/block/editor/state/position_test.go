package state

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestState_InsertTo(t *testing.T) {
	t.Run("default insert", func(t *testing.T) {
		r := New("root", nil)
		r.Add(simple.New(&model.Block{Id: "root"}))

		s := r.New()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("", 0, "first", "second")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Equal(t, []string{"first", "second"}, r.Pick("root").Model().ChildrenIds)
		assert.True(t, r.Exists("first"))
		assert.True(t, r.Exists("second"))
	})
	t.Run("bottom", func(t *testing.T) {
		r := New("root", nil)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target"}))

		s := r.New()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("target", model.Block_Bottom, "first", "second")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Equal(t, []string{"target", "first", "second"}, r.Pick("root").Model().ChildrenIds)
	})
	t.Run("top", func(t *testing.T) {
		r := New("root", nil)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target"}))

		s := r.New()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("target", model.Block_Top, "first", "second")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Equal(t, []string{"first", "second", "target"}, r.Pick("root").Model().ChildrenIds)
	})
	t.Run("inner", func(t *testing.T) {
		r := New("root", nil)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target"}))

		s := r.New()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("target", model.Block_Inner, "first", "second")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Equal(t, []string{"target"}, r.Pick("root").Model().ChildrenIds)
		assert.Equal(t, []string{"first", "second"}, r.Pick("target").Model().ChildrenIds)
	})
	t.Run("replace", func(t *testing.T) {
		r := New("root", nil)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target"}))

		s := r.New()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("target", model.Block_Replace, "first", "second")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 3)
		assert.Len(t, hist.Remove, 1)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
		assert.Equal(t, []string{"first", "second"}, r.Pick("root").Model().ChildrenIds)
	})

	moveFromSide := func(t *testing.T, pos model.BlockPosition) (r *State, c1, c2 simple.Block) {
		r = New("root", nil)
		r.Add(simple.New(&model.Block{Id: "root", ChildrenIds: []string{"target"}}))
		r.Add(simple.New(&model.Block{Id: "target"}))

		s := r.New()
		s.Add(simple.New(&model.Block{Id: "first"}))
		s.Add(simple.New(&model.Block{Id: "second"}))
		s.InsertTo("target", pos, "first", "second")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Remove, 0)
		assert.Len(t, hist.Add, 5) // 2 new + 2 columns + 1 row
		assert.Len(t, hist.Change, 1)

		require.Len(t, r.Pick("root").Model().ChildrenIds, 1)
		rowId := r.Pick("root").Model().ChildrenIds[0]
		row := r.Pick(rowId)
		assert.Equal(t, model.BlockContentLayout_Row, row.Model().GetLayout().Style)
		require.Len(t, row.Model().ChildrenIds, 2)
		c1 = r.Pick(row.Model().ChildrenIds[0])
		c2 = r.Pick(row.Model().ChildrenIds[1])
		return
	}

	t.Run("left to generic", func(t *testing.T) {
		_, c1, c2 := moveFromSide(t, model.Block_Left)
		assert.Len(t, c1.Model().ChildrenIds, 2)
		assert.Len(t, c2.Model().ChildrenIds, 1)
	})
	t.Run("right to generic", func(t *testing.T) {
		_, c1, c2 := moveFromSide(t, model.Block_Right)
		assert.Len(t, c1.Model().ChildrenIds, 1)
		assert.Len(t, c2.Model().ChildrenIds, 2)
	})
	t.Run("left to column", func(t *testing.T) {
		r, c1, _ := moveFromSide(t, model.Block_Left)
		s := r.New()
		s.Add(simple.New(&model.Block{Id: "third"}))
		s.InsertTo(c1.Model().Id, model.Block_Left, "third")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Remove, 0)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
	})
	t.Run("left to column 2", func(t *testing.T) {
		r, _, c2 := moveFromSide(t, model.Block_Left)
		s := r.New()
		s.Add(simple.New(&model.Block{Id: "third"}))
		s.InsertTo(c2.Model().Id, model.Block_Left, "third")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Remove, 0)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
	})
	t.Run("left to column 2", func(t *testing.T) {
		r, _, c2 := moveFromSide(t, model.Block_Left)
		s := r.New()
		s.Add(simple.New(&model.Block{Id: "third"}))
		s.InsertTo(c2.Model().Id, model.Block_Right, "third")

		msgs, hist, err := s.Apply()
		require.NoError(t, err)
		assert.Len(t, msgs, 2)
		assert.Len(t, hist.Remove, 0)
		assert.Len(t, hist.Add, 2)
		assert.Len(t, hist.Change, 1)
	})

	t.Run("cycle ref error", func(t *testing.T) {
		r := New("root", nil)
		r.Add(simple.New(&model.Block{Id: "root"}))

		s := r.New()
		s.Add(simple.New(&model.Block{Id: "1", ChildrenIds: []string{"2"}}))
		s.Add(simple.New(&model.Block{Id: "2", ChildrenIds: []string{"1"}}))
		s.Get("root").Model().ChildrenIds = []string{"1"}

		_, _, err := s.Apply()
		assert.Error(t, err)
	})
}
