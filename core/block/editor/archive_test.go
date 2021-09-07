package editor

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/collection"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock/smarttest"
	"github.com/stretchr/testify/require"
)

func NewArchiveTest() *Archive {
	sb := smarttest.New("root")

	return &Archive{
		SmartBlock: sb,
		Collection: collection.NewCollection(sb),
	}
}

func TestArchive_Archive(t *testing.T) {
	t.Run("archive", func(t *testing.T) {
		a := NewArchiveTest()
		require.NoError(t, a.Init(&smartblock.InitContext{}))

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("2"))

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 2)
		require.Equal(t, "2", s.Get(chIds[0]).Model().GetLink().TargetBlockId)
		require.Equal(t, "1", s.Get(chIds[1]).Model().GetLink().TargetBlockId)

	})
	t.Run("archive archived", func(t *testing.T) {
		a := NewArchiveTest()
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
		a := NewArchiveTest()
		require.NoError(t, a.Init(&smartblock.InitContext{}))

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("2"))

		require.NoError(t, a.RemoveObject("2"))
		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
	t.Run("unarchived", func(t *testing.T) {
		a := NewArchiveTest()
		require.NoError(t, a.Init(&smartblock.InitContext{}))
		require.NoError(t, a.AddObject("1"))
		require.EqualError(t, a.RemoveObject("2"), collection.ErrObjectNotFound.Error())

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
}
