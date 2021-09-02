package editor

import (
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchive_Archive(t *testing.T) {
	t.Run("archive", func(t *testing.T) {
		c := newCtrl()
		a := NewObjectLinksCollection(nil, c)
		a.SmartBlock = smarttest.New("root")
		require.NoError(t, a.Init(&smartblock.InitContext{}))

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("2"))

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 2)
		require.Equal(t, "2", s.Get(chIds[0]).Model().GetLink().TargetBlockId)
		require.Equal(t, "1", s.Get(chIds[1]).Model().GetLink().TargetBlockId)
		assert.True(t, c.values["1"])
		assert.True(t, c.values["2"])
	})
	t.Run("archive archived", func(t *testing.T) {
		c := newCtrl()
		a := NewObjectLinksCollection(nil, c)
		a.SmartBlock = smarttest.New("root")
		require.NoError(t, a.Init(&smartblock.InitContext{}))

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("1"))

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
}

func TestArchive_UnArchive(t *testing.T) {
	t.Run("unarchive", func(t *testing.T) {
		c := newCtrl()
		a := NewObjectLinksCollection(nil, c)
		a.SmartBlock = smarttest.New("root")
		require.NoError(t, a.Init(&smartblock.InitContext{}))

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("2"))

		require.NoError(t, a.RemoveObject("2"))
		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
		assert.True(t, c.values["1"])
		assert.False(t, c.values["2"])
	})
	t.Run("unarchived", func(t *testing.T) {
		c := newCtrl()
		a := NewObjectLinksCollection(nil, c)
		a.SmartBlock = smarttest.New("root")
		require.NoError(t, a.Init(&smartblock.InitContext{}))

		require.NoError(t, a.AddObject("1"))

		require.NoError(t, a.RemoveObject("2"))

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
}

func TestArchive_Delete(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		c := newCtrl()
		a := NewObjectLinksCollection(nil, c)
		a.SmartBlock = smarttest.New("root")
		require.NoError(t, a.Init(&smartblock.InitContext{}))

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("2"))

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 2)
		require.Equal(t, "2", s.Get(chIds[0]).Model().GetLink().TargetBlockId)
		require.Equal(t, "1", s.Get(chIds[1]).Model().GetLink().TargetBlockId)
		assert.True(t, c.values["1"])
		assert.True(t, c.values["2"])

		require.NoError(t, a.Delete("1"))
		require.NoError(t, a.Delete("2"))

		assert.Len(t, c.values, 0)
	})
}

func newCtrl() *ctrl {
	return &ctrl{values: make(map[string]bool)}
}

type ctrl struct {
	values map[string]bool
}

func (c *ctrl) MarkArchived(id string, archived bool) (err error) {
	c.values[id] = archived
	return nil
}

func (c *ctrl) DeleteArchivedObject(id string) (err error) {
	delete(c.values, id)
	return nil
}
