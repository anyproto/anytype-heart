package editor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestArchive_ReconcileInStore(t *testing.T) {
	// the editors are built without Init so the hook does not spawn a racing background reconcile
	t.Run("archive writes marker after successful reconcile", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		store := spaceindex.NewStoreFixture(t)
		a := &Archive{
			SmartBlock:  sb,
			Collection:  blockcollection.NewCollection(sb, store),
			objectStore: store,
		}
		want := spaceindex.HashIds([]string{"head1"})

		// when
		a.reconcileInStore([]string{"obj1", "obj2"}, want)

		// then
		got, err := store.GetReconcileMarker(context.Background(), "root")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("empty marker is not persisted", func(t *testing.T) {
		// given
		sb := smarttest.New("root")
		store := spaceindex.NewStoreFixture(t)
		a := &Archive{
			SmartBlock:  sb,
			Collection:  blockcollection.NewCollection(sb, store),
			objectStore: store,
		}

		// when: an object without a tree (virtual, tests) yields an empty marker
		a.reconcileInStore([]string{"obj1"}, "")

		// then
		got, err := store.GetReconcileMarker(context.Background(), "root")
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("dashboard writes marker after successful reconcile", func(t *testing.T) {
		// given
		sb := smarttest.New("home")
		store := spaceindex.NewStoreFixture(t)
		d := &Dashboard{
			SmartBlock:  sb,
			Collection:  blockcollection.NewCollection(sb, store),
			objectStore: store,
		}
		want := spaceindex.HashIds([]string{"head1"})

		// when
		d.reconcileInStore([]string{"fav1"}, want)

		// then
		got, err := store.GetReconcileMarker(context.Background(), "home")
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}
