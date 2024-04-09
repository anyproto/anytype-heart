package editor

import (
	"testing"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree/mock_objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/testMock"
)

func TestSpaceView_AccessType(t *testing.T) {
	t.Run("personal", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish()
		err := fx.SetAccessType(spaceinfo.AccessTypePersonal)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		err = fx.SetAccessType(spaceinfo.AccessTypeShared)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		err = fx.SetAccessType(spaceinfo.AccessTypePrivate)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		err = fx.SetAclIsEmpty(false)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetShareableStatus(spaceinfo.ShareableStatusShareable)
		err = fx.SetSpaceLocalInfo(info)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePersonal, fx.getAccessType())
	})
	t.Run("private->shareable", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish()
		err := fx.SetAccessType(spaceinfo.AccessTypePrivate)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePrivate, fx.getAccessType())
		err = fx.SetAccessType(spaceinfo.AccessTypeShared)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
		err = fx.SetAccessType(spaceinfo.AccessTypePrivate)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePrivate, fx.getAccessType())
		err = fx.SetAclIsEmpty(false)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetShareableStatus(spaceinfo.ShareableStatusNotShareable)
		err = fx.SetSpaceLocalInfo(info)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
		err = fx.SetAclIsEmpty(true)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypePrivate, fx.getAccessType())
		info.SetShareableStatus(spaceinfo.ShareableStatusShareable)
		err = fx.SetSpaceLocalInfo(info)
		require.NoError(t, err)
		require.Equal(t, spaceinfo.AccessTypeShared, fx.getAccessType())
	})
}

type spaceServiceStub struct {
}

func (s *spaceServiceStub) OnViewUpdated(info spaceinfo.SpacePersistentInfo) {
}

func (s *spaceServiceStub) OnWorkspaceChanged(spaceId string, details *types.Struct) {
}

func NewSpaceViewTest(t *testing.T, ctrl *gomock.Controller, targetSpaceId string, tree *mock_objecttree.MockObjectTree) (*SpaceView, error) {
	sb := smarttest.NewWithTree("root", tree)
	objectStore := testMock.NewMockObjectStore(ctrl)
	objectStore.EXPECT().GetDetails(gomock.Any()).AnyTimes()
	objectStore.EXPECT().Query(gomock.Any()).AnyTimes()
	a := &SpaceView{
		SmartBlock:   sb,
		spaceService: &spaceServiceStub{},
		log:          log,
	}

	initCtx := &smartblock.InitContext{
		IsNewObject: true,
	}
	changePayload := &model.ObjectChangePayload{
		Key: targetSpaceId,
	}
	marshaled, err := changePayload.Marshal()
	require.NoError(t, err)
	changeInfo := &treechangeproto.TreeChangeInfo{
		ChangePayload: marshaled,
	}
	tree.EXPECT().ChangeInfo().Return(changeInfo)
	if err := a.Init(initCtx); err != nil {
		return nil, err
	}
	migration.RunMigrations(a, initCtx)
	if err := a.Apply(initCtx.State); err != nil {
		return nil, err
	}
	return a, nil
}

type fixture struct {
	*SpaceView
	objectTree *mock_objecttree.MockObjectTree
	ctrl       *gomock.Controller
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	objectTree := mock_objecttree.NewMockObjectTree(ctrl)
	a, err := NewSpaceViewTest(t, ctrl, "spaceId", objectTree)
	require.NoError(t, err)
	return &fixture{
		SpaceView:  a,
		objectTree: objectTree,
		ctrl:       ctrl,
	}
}

func (f *fixture) getAccessType() spaceinfo.AccessType {
	return spaceinfo.AccessType(pbtypes.GetInt64(f.CombinedDetails(), bundle.RelationKeySpaceAccessType.String()))
}

func (f *fixture) finish() {
	f.ctrl.Finish()
}
