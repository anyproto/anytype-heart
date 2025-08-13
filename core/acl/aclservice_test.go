package acl

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient/mock_aclclient"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/list/mock_list"
	"github.com/anyproto/any-sync/commonspace/object/acl/recordverifier"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl/headupdater"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/commonspace/sync/syncdeps"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/anyproto/any-sync/consensus/consensusproto"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/cidutil"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/cheggaaa/mb/v3"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/inviteservice"
	"github.com/anyproto/anytype-heart/core/inviteservice/mock_inviteservice"
	"github.com/anyproto/anytype-heart/pb"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub/mock_crossspacesub"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

type mockConfig struct {
	Config nodeconf.Configuration
}

func (c *mockConfig) Name() string {
	return ""
}

func (c *mockConfig) Init(*app.App) (err error) {
	return nil
}

func (c *mockConfig) GetNodeConf() (conf nodeconf.Configuration) {
	return c.Config
}

type fixture struct {
	*aclService
	a                       *app.App
	ctrl                    *gomock.Controller
	mockJoiningClient       *mock_aclclient.MockAclJoiningClient
	mockSpaceService        *mock_space.MockService
	mockSubscriptionService *mock_subscription.MockService
	mockAccountService      *mock_account.MockService
	mockInviteService       *mock_inviteservice.MockInviteService
	mockCoordinatorClient   *mock_coordinatorclient.MockCoordinatorClient
	mockTechSpace           *mock_techspace.MockTechSpace
	mockSpaceView           *mock_techspace.MockSpaceView
	mockClientSpace         *mock_clientspace.MockSpace
	mockCommonSpace         *mock_commonspace.MockSpace
	mockSpaceClient         *mock_aclclient.MockAclSpaceClient
	mockCrossSpace          *mock_crossspacesub.MockService
	mockWallet              *mock_wallet.MockWallet
	mockAcl                 *mock_list.MockAclList
	mockConfig              *mockConfig
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		aclService:              New().(*aclService),
		a:                       new(app.App),
		ctrl:                    ctrl,
		mockJoiningClient:       mock_aclclient.NewMockAclJoiningClient(ctrl),
		mockSpaceService:        mock_space.NewMockService(t),
		mockSubscriptionService: mock_subscription.NewMockService(t),
		mockAccountService:      mock_account.NewMockService(t),
		mockInviteService:       mock_inviteservice.NewMockInviteService(t),
		mockCoordinatorClient:   mock_coordinatorclient.NewMockCoordinatorClient(ctrl),
		mockTechSpace:           mock_techspace.NewMockTechSpace(t),
		mockSpaceView:           mock_techspace.NewMockSpaceView(t),
		mockCrossSpace:          mock_crossspacesub.NewMockService(t),
		mockWallet:              mock_wallet.NewMockWallet(t),
		mockClientSpace:         mock_clientspace.NewMockSpace(t),
		mockCommonSpace:         mock_commonspace.NewMockSpace(ctrl),
		mockSpaceClient:         mock_aclclient.NewMockAclSpaceClient(ctrl),
		mockAcl:                 mock_list.NewMockAclList(ctrl),
		mockConfig:              &mockConfig{},
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.mockAccountService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockWallet)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockJoiningClient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockSpaceService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockInviteService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockSubscriptionService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockCoordinatorClient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockCrossSpace)).
		Register(fx.mockConfig).
		Register(fx.aclService)
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	fx.mockSpaceService.EXPECT().TechSpaceId().Return("techSpaceId")
	fx.mockWallet.EXPECT().Account().Return(keys)
	fx.mockCrossSpace.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscriptionservice.SubscribeResponse{}, nil)
	events := mb.New[*pb.EventMessage](0)
	fx.mockSubscriptionService.EXPECT().Search(mock.Anything).Return(&subscriptionservice.SubscribeResponse{
		Records: []*domain.Details{},
		Output:  events,
	}, nil)
	fx.mockJoiningClient.EXPECT().AclGetRecords(ctx, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("no acl found")).AnyTimes()
	require.NoError(t, fx.a.Start(ctx))
	fx.aclService.recordVerifier = recordverifier.NewValidateFull()
	return fx
}

func (fx *fixture) finish(t *testing.T) {
	fx.ctrl.Finish()
}

var _ syncacl.SyncAcl = (*mockSyncAcl)(nil)

type mockSyncAcl struct {
	list.AclList
}

func (m mockSyncAcl) HandleMessage(ctx context.Context, senderId string, protoVersion uint32, message *spacesyncproto.ObjectSyncMessage) (err error) {
	return
}

func (m mockSyncAcl) SyncWithPeer(ctx context.Context, p peer.Peer) (err error) {
	return
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

func (m mockSyncAcl) HandleRequest(ctx context.Context, senderId string, request *spacesyncproto.ObjectSyncMessage) (response *spacesyncproto.ObjectSyncMessage, err error) {
	return nil, nil
}

func (m mockSyncAcl) SetAclUpdater(updater headupdater.AclUpdater) {
}

func (m mockSyncAcl) HandleHeadUpdate(ctx context.Context, statusUpdater syncstatus.StatusUpdater, headUpdate drpc.Message) (syncdeps.Request, error) {
	return nil, nil
}
func (m mockSyncAcl) HandleStreamRequest(ctx context.Context, rq syncdeps.Request, updater syncdeps.QueueSizeUpdater, send func(resp proto.Message) error) (syncdeps.Request, error) {
	return nil, nil
}
func (m mockSyncAcl) HandleDeprecatedRequest(ctx context.Context, req *spacesyncproto.ObjectSyncMessage) (resp *spacesyncproto.ObjectSyncMessage, err error) {
	return nil, nil
}

func (m mockSyncAcl) HandleResponse(ctx context.Context, peerId string, objectId string, resp syncdeps.Response) error {
	return nil
}
func (m mockSyncAcl) ResponseCollector() syncdeps.ResponseCollector {
	return nil
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

func TestService_ViewInvite(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		keys, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.mockAccountService.EXPECT().Keys().Return(keys)
		aclList, err := list.NewInMemoryDerivedAcl("spaceId", keys)
		require.NoError(t, err)
		inv, err := aclList.RecordBuilder().BuildInvite()
		require.NoError(t, err)
		err = aclList.AddRawRecord(list.WrapAclRecord(inv.InviteRec))
		require.NoError(t, err)
		recs, err := aclList.RecordsAfter(ctx, "")
		require.NoError(t, err)
		cidString, err := cidutil.NewCidFromBytes([]byte("spaceId"))
		require.NoError(t, err)
		realCid, err := cid.Decode(cidString)
		require.NoError(t, err)
		protoKey, err := inv.InviteKey.Marshall()
		require.NoError(t, err)
		symKey, err := crypto.NewRandomAES()
		require.NoError(t, err)
		fx.mockInviteService.EXPECT().View(ctx, realCid, symKey).Return(domain.InviteView{
			AclKey:  protoKey,
			SpaceId: "spaceId",
		}, nil)
		fx.mockJoiningClient.EXPECT().AclGetRecords(ctx, "spaceId", "").Return(recs, nil)
		invite, err := fx.ViewInvite(ctx, realCid, symKey)
		require.NoError(t, err)
		require.Equal(t, "spaceId", invite.SpaceId)
	})
	t.Run("fail", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		keys, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.mockAccountService.EXPECT().Keys().Return(keys)
		aclList, err := list.NewInMemoryDerivedAcl("spaceId", keys)
		require.NoError(t, err)
		inv, err := aclList.RecordBuilder().BuildInvite()
		require.NoError(t, err)
		err = aclList.AddRawRecord(list.WrapAclRecord(inv.InviteRec))
		require.NoError(t, err)
		invRecIds := aclList.AclState().InviteIds()
		removeInv, err := aclList.RecordBuilder().BuildBatchRequest(list.BatchRequestPayload{
			InviteRevokes: invRecIds,
		})
		require.NoError(t, err)
		err = aclList.AddRawRecord(list.WrapAclRecord(removeInv.Rec))
		require.NoError(t, err)
		recs, err := aclList.RecordsAfter(ctx, "")
		require.NoError(t, err)
		cidString, err := cidutil.NewCidFromBytes([]byte("spaceId"))
		require.NoError(t, err)
		realCid, err := cid.Decode(cidString)
		require.NoError(t, err)
		protoKey, err := inv.InviteKey.Marshall()
		require.NoError(t, err)
		symKey, err := crypto.NewRandomAES()
		require.NoError(t, err)
		fx.mockInviteService.EXPECT().View(ctx, realCid, symKey).Return(domain.InviteView{
			AclKey:  protoKey,
			SpaceId: "spaceId",
		}, nil)
		fx.mockJoiningClient.EXPECT().AclGetRecords(ctx, "spaceId", "").Return(recs, nil)
		_, err = fx.ViewInvite(ctx, realCid, symKey)
		require.Equal(t, inviteservice.ErrInviteNotExists, err)
	})
}

func TestService_ChangeInvite(t *testing.T) {
	t.Run("change invite", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		mockSpace := mock_clientspace.NewMockSpace(t)
		mockCommonSpace := mock_commonspace.NewMockSpace(fx.ctrl)
		fx.mockInviteService.EXPECT().GetCurrent(ctx, spaceId).Return(domain.InviteInfo{
			InviteType:    domain.InviteTypeAnyone,
			InviteFileCid: "testCid",
		}, nil)
		fx.mockSpaceService.EXPECT().Get(ctx, spaceId).Return(mockSpace, nil)
		mockSpace.EXPECT().CommonSpace().Return(mockCommonSpace)
		exec := list.NewAclExecutor(spaceId)
		type cmdErr struct {
			cmd string
			err error
		}
		cmds := []cmdErr{
			{"a.init::a", nil},
			{"a.invite_anyone::invId,r", nil},
		}
		for _, cmd := range cmds {
			err := exec.Execute(cmd.cmd)
			require.Equal(t, cmd.err, err, cmd)
		}
		acl := mockSyncAcl{exec.ActualAccounts()["a"].Acl}
		invId := acl.AclState().Invites(aclrecordproto.AclInviteType_AnyoneCanJoin)[0].Id
		mockCommonSpace.EXPECT().Acl().Return(acl)
		aclClient := mock_aclclient.NewMockAclSpaceClient(fx.ctrl)
		mockCommonSpace.EXPECT().AclClient().Return(aclClient)
		aclClient.EXPECT().ChangeInvitePermissions(gomock.Any(), invId, list.AclPermissionsWriter).Return(nil)
		fx.mockInviteService.EXPECT().Change(ctx, spaceId, list.AclPermissionsWriter).Return(nil)
		err := fx.ChangeInvite(ctx, spaceId, model.ParticipantPermissions_Writer)
		require.NoError(t, err)
	})
	t.Run("different invite type", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockInviteService.EXPECT().GetCurrent(ctx, spaceId).Return(domain.InviteInfo{
			InviteType:    domain.InviteTypeDefault,
			InviteFileCid: "testCid",
		}, nil)
		err := fx.ChangeInvite(ctx, spaceId, model.ParticipantPermissions_Writer)
		require.Equal(t, inviteservice.ErrInviteNotExists, err)
	})
}

func TestService_GenerateInvite(t *testing.T) {
	t.Run("new default invite", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		keys, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockInviteService.EXPECT().GetCurrent(ctx, spaceId).Return(domain.InviteInfo{}, inviteservice.ErrInviteNotExists)
		mockSpace := mock_clientspace.NewMockSpace(t)
		mockCommonSpace := mock_commonspace.NewMockSpace(fx.ctrl)
		mockAclClient := mock_aclclient.NewMockAclSpaceClient(fx.ctrl)
		mockSpace.EXPECT().CommonSpace().Return(mockCommonSpace)
		mockCommonSpace.EXPECT().AclClient().Return(mockAclClient)
		fx.mockSpaceService.EXPECT().Get(ctx, spaceId).Return(mockSpace, nil)
		rec := &consensusproto.RawRecord{
			Payload: []byte("test"),
		}
		mockAclClient.EXPECT().ReplaceInvite(gomock.Any(), gomock.Any()).
			Return(list.InviteResult{
				InviteRec: rec,
				InviteKey: keys.SignKey,
			}, nil)
		params := inviteservice.GenerateInviteParams{
			SpaceId:     spaceId,
			InviteType:  domain.InviteTypeDefault,
			Key:         keys.SignKey,
			Permissions: list.AclPermissionsReader,
		}
		mockAclClient.EXPECT().AddRecord(ctx, rec).Return(nil)
		fx.mockInviteService.EXPECT().Generate(ctx, params, mock.Anything).
			RunAndReturn(func(ctx2 context.Context, params inviteservice.GenerateInviteParams, f func() error) (domain.InviteInfo, error) {
				return domain.InviteInfo{
					InviteFileCid: "testCid",
				}, f()
			})
		info, err := fx.GenerateInvite(ctx, spaceId, model.InviteType_Member, model.ParticipantPermissions_Reader)
		require.NoError(t, err)
		require.Equal(t, "testCid", info.InviteFileCid)
	})
	t.Run("new anyone can join invite", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		keys, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockInviteService.EXPECT().GetCurrent(ctx, spaceId).Return(domain.InviteInfo{}, inviteservice.ErrInviteNotExists)
		mockSpace := mock_clientspace.NewMockSpace(t)
		mockCommonSpace := mock_commonspace.NewMockSpace(fx.ctrl)
		mockAclClient := mock_aclclient.NewMockAclSpaceClient(fx.ctrl)
		mockSpace.EXPECT().CommonSpace().Return(mockCommonSpace)
		mockCommonSpace.EXPECT().AclClient().Return(mockAclClient)
		fx.mockSpaceService.EXPECT().Get(ctx, spaceId).Return(mockSpace, nil)
		rec := &consensusproto.RawRecord{
			Payload: []byte("test"),
		}
		mockAclClient.EXPECT().ReplaceInvite(gomock.Any(), gomock.Any()).
			Return(list.InviteResult{
				InviteRec: rec,
				InviteKey: keys.SignKey,
			}, nil)
		params := inviteservice.GenerateInviteParams{
			SpaceId:     spaceId,
			InviteType:  domain.InviteTypeAnyone,
			Key:         keys.SignKey,
			Permissions: list.AclPermissionsReader,
		}
		mockAclClient.EXPECT().AddRecord(ctx, rec).Return(nil)
		fx.mockInviteService.EXPECT().Generate(ctx, params, mock.Anything).
			RunAndReturn(func(ctx2 context.Context, params inviteservice.GenerateInviteParams, f func() error) (domain.InviteInfo, error) {
				return domain.InviteInfo{
					InviteFileCid: "testCid",
				}, f()
			})
		info, err := fx.GenerateInvite(ctx, spaceId, model.InviteType_WithoutApprove, model.ParticipantPermissions_Reader)
		require.NoError(t, err)
		require.Equal(t, "testCid", info.InviteFileCid)
	})
	t.Run("anyone can join invite after default invite", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		keys, err := accountdata.NewRandom()
		require.NoError(t, err)
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockInviteService.EXPECT().GetCurrent(ctx, spaceId).Return(domain.InviteInfo{
			InviteType: domain.InviteTypeDefault,
		}, nil)
		mockSpace := mock_clientspace.NewMockSpace(t)
		mockCommonSpace := mock_commonspace.NewMockSpace(fx.ctrl)
		mockAclClient := mock_aclclient.NewMockAclSpaceClient(fx.ctrl)
		mockSpace.EXPECT().CommonSpace().Return(mockCommonSpace)
		mockCommonSpace.EXPECT().AclClient().Return(mockAclClient)
		fx.mockSpaceService.EXPECT().Get(ctx, spaceId).Return(mockSpace, nil)
		rec := &consensusproto.RawRecord{
			Payload: []byte("test"),
		}
		mockAclClient.EXPECT().ReplaceInvite(gomock.Any(), gomock.Any()).
			Return(list.InviteResult{
				InviteRec: rec,
				InviteKey: keys.SignKey,
			}, nil)
		params := inviteservice.GenerateInviteParams{
			SpaceId:     spaceId,
			InviteType:  domain.InviteTypeAnyone,
			Key:         keys.SignKey,
			Permissions: list.AclPermissionsReader,
		}
		mockAclClient.EXPECT().AddRecord(ctx, rec).Return(nil)
		fx.mockInviteService.EXPECT().Generate(ctx, params, mock.Anything).
			RunAndReturn(func(ctx2 context.Context, params inviteservice.GenerateInviteParams, f func() error) (domain.InviteInfo, error) {
				return domain.InviteInfo{
					InviteFileCid: "testCid",
				}, f()
			})
		info, err := fx.GenerateInvite(ctx, spaceId, model.InviteType_WithoutApprove, model.ParticipantPermissions_Reader)
		require.NoError(t, err)
		require.Equal(t, "testCid", info.InviteFileCid)
	})
	t.Run("invite already exists", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		fx.mockAccountService.EXPECT().PersonalSpaceID().Return("personal")
		fx.mockInviteService.EXPECT().GetCurrent(ctx, spaceId).Return(domain.InviteInfo{
			InviteType:    domain.InviteTypeAnyone,
			InviteFileCid: "testCid",
		}, nil)
		info, err := fx.GenerateInvite(ctx, spaceId, model.InviteType_WithoutApprove, model.ParticipantPermissions_Reader)
		require.NoError(t, err)
		require.Equal(t, "testCid", info.InviteFileCid)
	})
}

func TestService_Join(t *testing.T) {
	t.Run("join success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		cidString, err := cidutil.NewCidFromBytes([]byte("spaceId"))
		require.NoError(t, err)
		realCid, err := cid.Decode(cidString)
		require.NoError(t, err)
		key, err := crypto.NewRandomAES()
		require.NoError(t, err)
		inviteKey, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		protoKey, err := inviteKey.Marshall()
		require.NoError(t, err)
		fx.mockInviteService.EXPECT().GetPayload(ctx, realCid, key).Return(&model.InvitePayload{
			AclKey: protoKey,
		}, nil)
		metadataPayload := []byte("metadata")
		fx.mockSpaceService.EXPECT().AccountMetadataPayload().Return(metadataPayload)
		fx.mockJoiningClient.EXPECT().RequestJoin(ctx, "spaceId", list.RequestJoinPayload{
			InviteKey: inviteKey,
			Metadata:  metadataPayload,
		}).Return("aclHeadId", nil)
		fx.mockSpaceService.EXPECT().Join(ctx, "spaceId", "aclHeadId").Return(nil)
		fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
		fx.mockTechSpace.EXPECT().SpaceViewSetData(ctx, "spaceId", mock.Anything).Return(nil)
		err = fx.Join(ctx, "spaceId", "", realCid, key)
		require.NoError(t, err)
	})
	t.Run("join fail, space is deleted", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		cidString, err := cidutil.NewCidFromBytes([]byte("spaceId"))
		require.NoError(t, err)
		realCid, err := cid.Decode(cidString)
		require.NoError(t, err)
		key, err := crypto.NewRandomAES()
		require.NoError(t, err)
		inviteKey, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		protoKey, err := inviteKey.Marshall()
		require.NoError(t, err)
		fx.mockInviteService.EXPECT().GetPayload(ctx, realCid, key).Return(&model.InvitePayload{
			AclKey: protoKey,
		}, nil)
		metadataPayload := []byte("metadata")
		fx.mockSpaceService.EXPECT().AccountMetadataPayload().Return(metadataPayload)
		fx.mockJoiningClient.EXPECT().RequestJoin(ctx, "spaceId", list.RequestJoinPayload{
			InviteKey: inviteKey,
			Metadata:  metadataPayload,
		}).Return("", coordinatorproto.ErrSpaceIsDeleted)
		err = fx.Join(ctx, "spaceId", "", realCid, key)
		require.Equal(t, space.ErrSpaceDeleted, err)
	})
	t.Run("join success, already member", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		cidString, err := cidutil.NewCidFromBytes([]byte("spaceId"))
		require.NoError(t, err)
		realCid, err := cid.Decode(cidString)
		require.NoError(t, err)
		key, err := crypto.NewRandomAES()
		require.NoError(t, err)
		inviteKey, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		protoKey, err := inviteKey.Marshall()
		require.NoError(t, err)
		fx.mockInviteService.EXPECT().GetPayload(ctx, realCid, key).Return(&model.InvitePayload{
			AclKey: protoKey,
		}, nil)
		metadataPayload := []byte("metadata")
		fx.mockSpaceService.EXPECT().AccountMetadataPayload().Return(metadataPayload)
		fx.mockJoiningClient.EXPECT().RequestJoin(ctx, "spaceId", list.RequestJoinPayload{
			InviteKey: inviteKey,
			Metadata:  metadataPayload,
		}).Return("", list.ErrInsufficientPermissions)
		fx.mockJoiningClient.EXPECT().CancelRemoveSelf(ctx, "spaceId").Return(nil)
		fx.mockSpaceService.EXPECT().CancelLeave(ctx, "spaceId").Return(nil)
		err = fx.Join(ctx, "spaceId", "", realCid, key)
		require.NoError(t, err)
	})
	t.Run("join success without approve", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		cidString, err := cidutil.NewCidFromBytes([]byte("spaceId"))
		require.NoError(t, err)
		realCid, err := cid.Decode(cidString)
		require.NoError(t, err)
		key, err := crypto.NewRandomAES()
		require.NoError(t, err)
		inviteKey, _, err := crypto.GenerateRandomEd25519KeyPair()
		require.NoError(t, err)
		protoKey, err := inviteKey.Marshall()
		require.NoError(t, err)
		fx.mockInviteService.EXPECT().GetPayload(ctx, realCid, key).Return(&model.InvitePayload{
			AclKey:     protoKey,
			InviteType: model.InviteType_WithoutApprove,
		}, nil)
		metadataPayload := []byte("metadata")
		fx.mockSpaceService.EXPECT().AccountMetadataPayload().Return(metadataPayload)
		fx.mockJoiningClient.EXPECT().InviteJoin(ctx, "spaceId", list.InviteJoinPayload{
			InviteKey: inviteKey,
			Metadata:  metadataPayload,
		}).Return("aclHeadId", nil)
		fx.mockSpaceService.EXPECT().InviteJoin(ctx, "spaceId", "aclHeadId").Return(nil)
		fx.mockSpaceService.EXPECT().TechSpace().Return(&clientspace.TechSpace{TechSpace: fx.mockTechSpace})
		fx.mockTechSpace.EXPECT().SpaceViewSetData(ctx, "spaceId", mock.Anything).Return(nil)
		err = fx.Join(ctx, "spaceId", "", realCid, key)
		require.NoError(t, err)
	})
	t.Run("join fail, different network", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockConfig.Config = nodeconf.Configuration{NetworkId: "net1"}

		// when
		err := fx.Join(ctx, "spaceId", "net2", cid.Cid{}, nil)

		// then
		require.True(t, errors.Is(err, ErrDifferentNetwork))
	})
}

func TestService_Leave(t *testing.T) {
	t.Run("leave success", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		
		// Mock ACL getter to return ACL records
		keys, err := accountdata.NewRandom()
		require.NoError(t, err)
		aclList, err := list.NewInMemoryDerivedAcl(spaceId, keys)
		require.NoError(t, err)
		
		// Get the raw records from the ACL to return them in the mock
		records, err := aclList.RecordsAfter(ctx, "")
		require.NoError(t, err)
		
		fx.mockJoiningClient.EXPECT().AclGetRecords(ctx, spaceId, "").Return(records, nil)
		fx.mockJoiningClient.EXPECT().RequestSelfRemove(ctx, spaceId, gomock.Any()).Return(nil)
		err = fx.Leave(ctx, spaceId)
		require.NoError(t, err)
	})
	t.Run("leave success if acl getter error is known", func(t *testing.T) {
		errs := []error{
			space.ErrSpaceStorageMissig,
			space.ErrSpaceDeleted,
		}
		for _, err := range errs {
			t.Run("known error "+err.Error(), func(t *testing.T) {
				fx := newFixture(t)
				defer fx.finish(t)
				spaceId := "spaceId"
				// Mock ACL getter to return a known error that should be handled gracefully
				fx.mockJoiningClient.EXPECT().AclGetRecords(ctx, spaceId, "").Return(nil, err)
				err = fx.Leave(ctx, spaceId)
				require.NoError(t, err)
			})
		}
	})
	t.Run("leave fail if acl getter error is unknown", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		// Mock ACL getter to return an unknown error that should be converted
		fx.mockJoiningClient.EXPECT().AclGetRecords(ctx, spaceId, "").Return(nil, fmt.Errorf("unknown error"))
		err := fx.Leave(ctx, spaceId)
		require.True(t, errors.Is(err, ErrAclRequestFailed))
	})
	t.Run("leave success if acl request error is known", func(t *testing.T) {
		errs := []error{
			list.ErrPendingRequest,
			list.ErrIsOwner,
			list.ErrNoSuchAccount,
			coordinatorproto.ErrSpaceIsDeleted,
			coordinatorproto.ErrSpaceNotExists,
		}
		for _, err := range errs {
			t.Run("known error "+err.Error(), func(t *testing.T) {
				fx := newFixture(t)
				defer fx.finish(t)
				spaceId := "spaceId"
				
				// Mock ACL getter to return ACL records
				keys, err := accountdata.NewRandom()
				require.NoError(t, err)
				aclList, err := list.NewInMemoryDerivedAcl(spaceId, keys)
				require.NoError(t, err)
				records, err := aclList.RecordsAfter(ctx, "")
				require.NoError(t, err)
				
				fx.mockJoiningClient.EXPECT().AclGetRecords(ctx, spaceId, "").Return(records, nil)
				fx.mockJoiningClient.EXPECT().RequestSelfRemove(ctx, spaceId, gomock.Any()).Return(err)
				
				err = fx.Leave(ctx, spaceId)
				require.NoError(t, err)
			})
		}
	})
	t.Run("leave fail if acl request error is unknown", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"
		
		// Mock ACL getter to return ACL records
		keys, err := accountdata.NewRandom()
		require.NoError(t, err)
		aclList, err := list.NewInMemoryDerivedAcl(spaceId, keys)
		require.NoError(t, err)
		records, err := aclList.RecordsAfter(ctx, "")
		require.NoError(t, err)
		
		fx.mockJoiningClient.EXPECT().AclGetRecords(ctx, spaceId, "").Return(records, nil)
		fx.mockJoiningClient.EXPECT().RequestSelfRemove(ctx, spaceId, gomock.Any()).Return(fmt.Errorf("unknown error"))
		
		err = fx.Leave(ctx, spaceId)
		require.True(t, errors.Is(err, ErrAclRequestFailed))
	})

	t.Run("leave if acl access fails", func(t *testing.T) {
		// this is a case of guest user or user without proper ACL access trying to leave the space
		fx := newFixture(t)
		defer fx.finish(t)
		spaceId := "spaceId"

		// Mock ACL getter to fail with "no such account" error (guest user scenario)
		fx.mockJoiningClient.EXPECT().AclGetRecords(ctx, spaceId, "").Return(nil, list.ErrNoSuchAccount)
		
		err := fx.Leave(ctx, spaceId)
		require.NoError(t, err)
	})
}
