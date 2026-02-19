package editor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/blockcollection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
)

func NewArchiveTest(t *testing.T) (*Archive, error) {
	sb := smarttest.New("root")
	objectStore := spaceindex.NewStoreFixture(t)
	a := &Archive{
		SmartBlock:  sb,
		Collection:  blockcollection.NewCollection(sb, objectStore),
		objectStore: objectStore,
	}

	initCtx := &smartblock.InitContext{
		IsNewObject: true,
	}
	if err := a.Init(initCtx); err != nil {
		return nil, err
	}
	migration.RunMigrations(a, initCtx)
	if err := a.Apply(initCtx.State); err != nil {
		return nil, err
	}
	return a, nil
}

func TestArchive_Archive(t *testing.T) {
	t.Run("archive", func(t *testing.T) {
		a, err := NewArchiveTest(t)
		require.NoError(t, err)

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("2"))

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 2)
		require.Equal(t, "2", s.Get(chIds[0]).Model().GetLink().TargetBlockId)
		require.Equal(t, "1", s.Get(chIds[1]).Model().GetLink().TargetBlockId)

	})
	t.Run("archive archived", func(t *testing.T) {
		a, err := NewArchiveTest(t)
		require.NoError(t, err)

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("1"))

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
}

func TestArchive_UnArchive(t *testing.T) {
	t.Run("unarchive", func(t *testing.T) {
		a, err := NewArchiveTest(t)
		require.NoError(t, err)

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("2"))

		require.NoError(t, a.RemoveObject("2"))
		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
	t.Run("unarchived", func(t *testing.T) {
		a, err := NewArchiveTest(t)
		require.NoError(t, err)

		require.NoError(t, a.AddObject("1"))
		require.EqualError(t, a.RemoveObject("2"), blockcollection.ErrObjectNotFound.Error())

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
}
