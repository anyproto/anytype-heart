package editor

import (
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchive_Archive(t *testing.T) {
	t.Run("archive", func(t *testing.T) {
		c := newCtrl()
		a := NewArchive(c)
		a.SmartBlock = smarttest.New("root").AddBlock(simple.New(&model.Block{Id: "root"}))
		require.NoError(t, a.Init(nil))

		require.NoError(t, a.Archive("1"))
		require.NoError(t, a.Archive("2"))

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
		a := NewArchive(c)
		a.SmartBlock = smarttest.New("root").AddBlock(simple.New(&model.Block{Id: "root"}))
		require.NoError(t, a.Init(nil))

		require.NoError(t, a.Archive("1"))
		require.NoError(t, a.Archive("1"))

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
}

func TestArchive_UnArchive(t *testing.T) {
	t.Run("unarchive", func(t *testing.T) {
		c := newCtrl()
		a := NewArchive(c)
		a.SmartBlock = smarttest.New("root").AddBlock(simple.New(&model.Block{Id: "root"}))
		require.NoError(t, a.Init(nil))

		require.NoError(t, a.Archive("1"))
		require.NoError(t, a.Archive("2"))

		require.NoError(t, a.UnArchive("2"))
		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
		assert.True(t, c.values["1"])
		assert.False(t, c.values["2"])
	})
	t.Run("unarchived", func(t *testing.T) {
		c := newCtrl()
		a := NewArchive(c)
		a.SmartBlock = smarttest.New("root").AddBlock(simple.New(&model.Block{Id: "root"}))
		require.NoError(t, a.Init(nil))

		require.NoError(t, a.Archive("1"))

		require.NoError(t, a.UnArchive("2"))

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
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
