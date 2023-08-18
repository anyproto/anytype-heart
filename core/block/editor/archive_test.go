package editor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/collection"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/util/testMock"
	"github.com/anyproto/anytype-heart/util/testMock/mockDetailsModifier"
)

func NewArchiveTest(ctrl *gomock.Controller) (*Archive, error) {
	sb := smarttest.New("root")
	objectStore := testMock.NewMockObjectStore(ctrl)
	objectStore.EXPECT().GetDetails(gomock.Any()).AnyTimes()
	objectStore.EXPECT().Query(gomock.Any(), gomock.Any()).AnyTimes()
	dm := mockDetailsModifier.NewMockDetailsModifier(ctrl)
	dm.EXPECT().ModifyLocalDetails(gomock.Any(), gomock.Any()).AnyTimes()
	a := &Archive{
		SmartBlock:      sb,
		DetailsModifier: dm,
		Collection:      collection.NewCollection(sb),
		objectStore:     objectStore,
	}

	initCtx := &smartblock.InitContext{IsNewObject: true}
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
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("archive", func(t *testing.T) {
		a, err := NewArchiveTest(ctrl)
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
		a, err := NewArchiveTest(ctrl)
		require.NoError(t, err)

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("1"))

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
}

func TestArchive_UnArchive(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("unarchive", func(t *testing.T) {
		a, err := NewArchiveTest(ctrl)
		require.NoError(t, err)

		require.NoError(t, a.AddObject("1"))
		require.NoError(t, a.AddObject("2"))

		require.NoError(t, a.RemoveObject("2"))
		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
	t.Run("unarchived", func(t *testing.T) {
		a, err := NewArchiveTest(ctrl)
		require.NoError(t, err)

		require.NoError(t, a.AddObject("1"))
		require.EqualError(t, a.RemoveObject("2"), collection.ErrObjectNotFound.Error())

		s := a.NewState()
		chIds := s.Get(s.RootId()).Model().ChildrenIds
		require.Len(t, chIds, 1)
	})
}
