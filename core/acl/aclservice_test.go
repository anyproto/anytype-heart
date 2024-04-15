package acl

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient/mock_aclclient"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/list/mock_list"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl/headupdater"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/cidutil"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/inviteservice/mock_inviteservice"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

type fixture struct {
	*aclService
	a                     *app.App
	ctrl                  *gomock.Controller
	mockJoiningClient     *mock_aclclient.MockAclJoiningClient
	mockSpaceService      *mock_space.MockService
	mockAccountService    *mock_account.MockService
	mockInviteService     *mock_inviteservice.MockInviteService
	mockCoordinatorClient *mock_coordinatorclient.MockCoordinatorClient
	mockTechSpace         *mock_techspace.MockTechSpace
	mockSpaceView         *mock_techspace.MockSpaceView
	mockClientSpace       *mock_clientspace.MockSpace
	mockCommonSpace       *mock_commonspace.MockSpace
	mockSpaceClient       *mock_aclclient.MockAclSpaceClient
	mockAcl               *mock_list.MockAclList
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		aclService:            New().(*aclService),
		a:                     new(app.App),
		ctrl:                  ctrl,
		mockJoiningClient:     mock_aclclient.NewMockAclJoiningClient(ctrl),
		mockSpaceService:      mock_space.NewMockService(t),
		mockAccountService:    mock_account.NewMockService(t),
		mockInviteService:     mock_inviteservice.NewMockInviteService(t),
		mockCoordinatorClient: mock_coordinatorclient.NewMockCoordinatorClient(ctrl),
		mockTechSpace:         mock_techspace.NewMockTechSpace(t),
		mockSpaceView:         mock_techspace.NewMockSpaceView(t),
		mockClientSpace:       mock_clientspace.NewMockSpace(t),
		mockCommonSpace:       mock_commonspace.NewMockSpace(ctrl),
		mockSpaceClient:       mock_aclclient.NewMockAclSpaceClient(ctrl),
		mockAcl:               mock_list.NewMockAclList(ctrl),
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.mockAccountService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockJoiningClient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockSpaceService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockInviteService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockCoordinatorClient)).
		Register(fx.aclService)
	require.NoError(t, fx.a.Start(ctx))
	return fx
}

func (fx *fixture) finish(t *testing.T) {
	fx.ctrl.Finish()
}

type mockSyncAcl struct {
	list.AclList
}

func (m mockSyncAcl) Init(a *app.App) (err error) {
	return nil
}

func (m mockSyncAcl) Name() (name string) {
	return "mockSyncAcl"
}

func (m mockSyncAcl) Run(ctx context.Context) (err error) {
	return nil
}

func (m mockSyncAcl) HandleMessage(ctx context.Context, senderId string, message *spacesyncproto.ObjectSyncMessage) (err error) {
	return nil
}

func (m mockSyncAcl) HandleRequest(ctx context.Context, senderId string, request *spacesyncproto.ObjectSyncMessage) (response *spacesyncproto.ObjectSyncMessage, err error) {
	return nil, nil
}

func (m mockSyncAcl) SetHeadUpdater(updater headupdater.HeadUpdater) {
}

func (m mockSyncAcl) SyncWithPeer(ctx context.Context, peerId string) (err error) {
	return nil
}

func (m mockSyncAcl) SetAclUpdater(updater headupdater.AclUpdater) {
}

func TestService_MakeShareable(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetShareableStatus(spaceinfo.ShareableStatusShareable)
		fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
		fx.mockTechSpace.EXPECT().SetLocalInfo(ctx, info).Return(nil)
		fx.mockCoordinatorClient.EXPECT().SpaceMakeShareable(ctx, "spaceId").Return(nil)
		err := fx.MakeShareable(ctx, "spaceId")
		require.NoError(t, err)
	})
	t.Run("fail", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetShareableStatus(spaceinfo.ShareableStatusShareable)
		fx.mockCoordinatorClient.EXPECT().SpaceMakeShareable(ctx, "spaceId").Return(ErrLimitReached)
		err := fx.MakeShareable(ctx, "spaceId")
		require.Error(t, err)
	})
}

func TestService_StopSharing(t *testing.T) {
	t.Run("retry when failed with acl head", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetShareableStatus(spaceinfo.ShareableStatusShareable)
		fx.mockSpaceService.EXPECT().Get(ctx, "spaceId").Return(fx.mockClientSpace, nil)
		fx.mockClientSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
		fx.mockSpaceView.EXPECT().GetLocalInfo().Return(info)
		fx.mockTechSpace.EXPECT().DoSpaceView(ctx, "spaceId", mock.Anything).RunAndReturn(
			func(ctx context.Context, spaceId string, f func(view techspace.SpaceView) error) error {
				return f(fx.mockSpaceView)
			})
		fx.mockCommonSpace.EXPECT().Acl().Return(mockSyncAcl{fx.mockAcl})
		fx.mockCommonSpace.EXPECT().AclClient().Return(fx.mockSpaceClient)
		fx.mockSpaceClient.EXPECT().StopSharing(ctx, gomock.Any()).Return(nil)
		fx.mockAcl.EXPECT().RLock().AnyTimes()
		fx.mockAcl.EXPECT().RUnlock().AnyTimes()
		fx.mockAcl.EXPECT().Head().Return(&list.AclRecord{Id: "headId"})
		fx.mockInviteService.EXPECT().RemoveExisting(ctx, "spaceId").Return(nil)
		sleepTime = time.Millisecond
		fx.mockCoordinatorClient.EXPECT().SpaceMakeUnshareable(ctx, "spaceId", "headId").Return(coordinatorproto.ErrAclHeadIsMissing)
		fx.mockCoordinatorClient.EXPECT().SpaceMakeUnshareable(ctx, "spaceId", "headId").Return(nil)
		info.SetShareableStatus(spaceinfo.ShareableStatusNotShareable)
		fx.mockTechSpace.EXPECT().SetLocalInfo(ctx, info).Return(nil)
		err := fx.StopSharing(ctx, "spaceId")
		require.NoError(t, err)
	})
	t.Run("not call make unshareable if not shareable", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		info := spaceinfo.NewSpaceLocalInfo("spaceId")
		info.SetShareableStatus(spaceinfo.ShareableStatusNotShareable)
		fx.mockSpaceService.EXPECT().Get(ctx, "spaceId").Return(fx.mockClientSpace, nil)
		fx.mockClientSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
		fx.mockSpaceView.EXPECT().GetLocalInfo().Return(info)
		fx.mockTechSpace.EXPECT().DoSpaceView(ctx, "spaceId", mock.Anything).RunAndReturn(
			func(ctx context.Context, spaceId string, f func(view techspace.SpaceView) error) error {
				return f(fx.mockSpaceView)
			})
		fx.mockCommonSpace.EXPECT().Acl().Return(mockSyncAcl{fx.mockAcl})
		fx.mockCommonSpace.EXPECT().AclClient().Return(fx.mockSpaceClient)
		fx.mockSpaceClient.EXPECT().StopSharing(ctx, gomock.Any()).Return(nil)
		fx.mockAcl.EXPECT().RLock().AnyTimes()
		fx.mockAcl.EXPECT().RUnlock().AnyTimes()
		fx.mockAcl.EXPECT().Head().Return(&list.AclRecord{Id: "headId"})
		fx.mockInviteService.EXPECT().RemoveExisting(ctx, "spaceId").Return(nil)
		err := fx.StopSharing(ctx, "spaceId")
		require.NoError(t, err)
	})
}

func TestService_ApproveLeave(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		mockSpace := mock_clientspace.NewMockSpace(t)
		mockCommonSpace := mock_commonspace.NewMockSpace(fx.ctrl)
		fx.mockSpaceService.EXPECT().Get(ctx, spaceId).Times(2).Return(mockSpace, nil)
		mockSpace.EXPECT().CommonSpace().Times(2).Return(mockCommonSpace)
		exec := list.NewAclExecutor(spaceId)
		type cmdErr struct {
			cmd string
			err error
		}
		cmds := []cmdErr{
			{"a.init::a", nil},
			{"a.invite::invId", nil},
			{"b.join::invId", nil},
			{"c.join::invId", nil},
			{"a.approve::b,r", nil},
			{"a.approve::c,r", nil},
			{"b.request_remove::b", nil},
			{"c.request_remove::c", nil},
		}
		for _, cmd := range cmds {
			err := exec.Execute(cmd.cmd)
			require.Equal(t, cmd.err, err, cmd)
		}
		identityB := exec.ActualAccounts()["b"].Keys.SignKey.GetPublic()
		identityC := exec.ActualAccounts()["c"].Keys.SignKey.GetPublic()
		acl := mockSyncAcl{exec.ActualAccounts()["a"].Acl}
		mockCommonSpace.EXPECT().Acl().Return(acl)
		aclClient := mock_aclclient.NewMockAclSpaceClient(fx.ctrl)
		mockCommonSpace.EXPECT().AclClient().Return(aclClient)
		aclClient.EXPECT().RemoveAccounts(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, payload list.AccountRemovePayload) error {
			require.Equal(t, []crypto.PubKey{identityB, identityC}, payload.Identities)
			return nil
		}).Return(nil)
		err := fx.ApproveLeave(ctx, spaceId, []crypto.PubKey{identityB, identityC})
		require.NoError(t, err)
	})
	t.Run("fail", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		mockSpace := mock_clientspace.NewMockSpace(t)
		mockCommonSpace := mock_commonspace.NewMockSpace(fx.ctrl)
		fx.mockSpaceService.EXPECT().Get(ctx, spaceId).Times(1).Return(mockSpace, nil)
		mockSpace.EXPECT().CommonSpace().Times(1).Return(mockCommonSpace)
		exec := list.NewAclExecutor(spaceId)
		type cmdErr struct {
			cmd string
			err error
		}
		cmds := []cmdErr{
			{"a.init::a", nil},
			{"a.invite::invId", nil},
			{"b.join::invId", nil},
			{"c.join::invId", nil},
			{"a.approve::b,r", nil},
			{"a.approve::c,r", nil},
			{"c.request_remove::c", nil},
		}
		for _, cmd := range cmds {
			err := exec.Execute(cmd.cmd)
			require.Equal(t, cmd.err, err, cmd)
		}
		identityB := exec.ActualAccounts()["b"].Keys.SignKey.GetPublic()
		identityC := exec.ActualAccounts()["c"].Keys.SignKey.GetPublic()
		acl := mockSyncAcl{exec.ActualAccounts()["a"].Acl}
		mockCommonSpace.EXPECT().Acl().Return(acl)
		err := fx.ApproveLeave(ctx, spaceId, []crypto.PubKey{identityB, identityC})
		require.Error(t, err)
	})
}

func TestService_Join(t *testing.T) {
	fx := newFixture(t)
	defer fx.finish(t)
	cid, err := cidutil.NewCidFromBytes([]byte("spaceId"))
	require.NoError(t, err)
	key, err := crypto.NewRandomAES()
	require.NoError(t, err)
	inviteKey, _, err := crypto.GenerateRandomEd25519KeyPair()
	require.NoError(t, err)
	protoKey, err := inviteKey.Marshall()
	require.NoError(t, err)
	fx.mockInviteService.EXPECT().GetPayload(ctx, cid, key).Return(&model.InvitePayload{
		InviteKey: protoKey,
	}, nil)
	metadataPayload := []byte("metadata")
	fx.mockJoiningClient.EXPECT().RequestJoin(ctx, "spaceId", list.RequestJoinPayload{
		InviteKey: inviteKey,
		Metadata:  metadataPayload,
	}).Return("aclHeadId", nil)
	fx.mockSpaceService.EXPECT().Join(ctx, "spaceId", "aclHeadId").Return(nil)
	fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
	fx.mockTechSpace.EXPECT().SpaceViewSetData(ctx, "spaceId", mock.Anything).Return(nil)
	fx.Join(ctx, "space", cid, key)
}
