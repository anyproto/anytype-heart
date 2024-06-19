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
		fx := newSpaceViewFixture(t)
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
		fx := newSpaceViewFixture(t)
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

func TestSpaceView_Info(t *testing.T) {
	t.Run("local", func(t *testing.T) {
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		firstLocalInfo := fx.GetLocalInfo()
		require.Equal(t, spaceinfo.LocalStatusUnknown, firstLocalInfo.GetLocalStatus())
		require.Equal(t, spaceinfo.RemoteStatusUnknown, firstLocalInfo.GetRemoteStatus())
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetLocalStatus(spaceinfo.LocalStatusOk).
			SetRemoteStatus(spaceinfo.RemoteStatusOk).
			SetReadLimit(10).
			SetWriteLimit(10).
			SetShareableStatus(spaceinfo.ShareableStatusShareable)
		err := fx.SetSpaceLocalInfo(info)
		require.NoError(t, err)
		curInfo := fx.GetLocalInfo()
		require.Equal(t, spaceinfo.LocalStatusOk, curInfo.GetLocalStatus())
		require.Equal(t, spaceinfo.RemoteStatusOk, curInfo.GetRemoteStatus())
		require.Equal(t, uint32(10), curInfo.GetReadLimit())
		require.Equal(t, uint32(10), curInfo.GetWriteLimit())
		require.Equal(t, spaceinfo.ShareableStatusShareable, curInfo.GetShareableStatus())
	})
	t.Run("persistent", func(t *testing.T) {
		fx := newSpaceViewFixture(t)
		defer fx.finish()
		info := spaceinfo.NewSpacePersistentInfo("spaceId")
		info.SetAccountStatus(spaceinfo.AccountStatusActive)
		err := fx.SetSpacePersistentInfo(info)
		require.NoError(t, err)
		curInfo := fx.GetPersistentInfo()
		require.Equal(t, spaceinfo.AccountStatusActive, curInfo.GetAccountStatus())
		info = spaceinfo.NewSpacePersistentInfo("spaceId")
		info.SetAclHeadId("aclHeadId")
		err = fx.SetSpacePersistentInfo(info)
		require.NoError(t, err)
		curInfo = fx.GetPersistentInfo()
		require.Equal(t, "aclHeadId", curInfo.GetAclHeadId())
		require.Equal(t, spaceinfo.AccountStatusActive, curInfo.GetAccountStatus())
	})
}

func TestSpaceView_SharedSpacesLimit(t *testing.T) {
	fx := newSpaceViewFixture(t)
	defer fx.finish()
	err := fx.SetSharedSpacesLimit(10)
	require.NoError(t, err)
	require.Equal(t, 10, fx.GetSharedSpacesLimit())
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

type spaceViewFixture struct {
	*SpaceView
	objectTree *mock_objecttree.MockObjectTree
	ctrl       *gomock.Controller
}

func newSpaceViewFixture(t *testing.T) *spaceViewFixture {
	ctrl := gomock.NewController(t)
	objectTree := mock_objecttree.NewMockObjectTree(ctrl)
	a, err := NewSpaceViewTest(t, ctrl, "spaceId", objectTree)
	require.NoError(t, err)
	return &spaceViewFixture{
		SpaceView:  a,
		objectTree: objectTree,
		ctrl:       ctrl,
	}
}

func (f *spaceViewFixture) getAccessType() spaceinfo.AccessType {
	return spaceinfo.AccessType(pbtypes.GetInt64(f.CombinedDetails(), bundle.RelationKeySpaceAccessType.String()))
}

func (f *spaceViewFixture) finish() {
	f.ctrl.Finish()
}
