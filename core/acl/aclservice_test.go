package acl

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient/mock_aclclient"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl/headupdater"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/block/getblock/mock_getblock"
	"github.com/anyproto/anytype-heart/core/files/fileacl/mock_fileacl"
	"github.com/anyproto/anytype-heart/core/invitestore/mock_invitestore"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

type fixture struct {
	*aclService
	a                  *app.App
	ctrl               *gomock.Controller
	mockJoiningClient  *mock_aclclient.MockAclJoiningClient
	mockSpaceService   *mock_space.MockService
	mockAccountService *mock_account.MockService
	mockObjectGetter   *mock_getblock.MockObjectGetterComponent
	mockFileAcl        *mock_fileacl.MockService
	mockInviteStore    *mock_invitestore.MockService
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		aclService:         New().(*aclService),
		a:                  new(app.App),
		ctrl:               ctrl,
		mockJoiningClient:  mock_aclclient.NewMockAclJoiningClient(ctrl),
		mockSpaceService:   mock_space.NewMockService(t),
		mockAccountService: mock_account.NewMockService(t),
		mockObjectGetter:   mock_getblock.NewMockObjectGetterComponent(t),
		mockFileAcl:        mock_fileacl.NewMockService(t),
		mockInviteStore:    mock_invitestore.NewMockService(t),
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.mockAccountService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockJoiningClient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockSpaceService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockFileAcl)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockInviteStore)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockObjectGetter)).
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
			{"a.approve::b,r", nil},
			{"b.request_remove::b", nil},
		}
		for _, cmd := range cmds {
			err := exec.Execute(cmd.cmd)
			require.Equal(t, cmd.err, err, cmd)
		}
		identity := exec.ActualAccounts()["b"].Keys.SignKey.GetPublic()
		acl := mockSyncAcl{exec.ActualAccounts()["b"].Acl}
		mockCommonSpace.EXPECT().Acl().Return(acl)
		aclClient := mock_aclclient.NewMockAclSpaceClient(fx.ctrl)
		mockCommonSpace.EXPECT().AclClient().Return(aclClient)
		aclClient.EXPECT().RemoveAccounts(ctx, gomock.Any()).DoAndReturn(func(ctx context.Context, payload list.AccountRemovePayload) error {
			require.Equal(t, []crypto.PubKey{identity}, payload.Identities)
			return nil
		}).Return(nil)
		err := fx.ApproveLeave(ctx, spaceId, identity)
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
			{"a.approve::b,r", nil},
		}
		for _, cmd := range cmds {
			err := exec.Execute(cmd.cmd)
			require.Equal(t, cmd.err, err, cmd)
		}
		identity := exec.ActualAccounts()["b"].Keys.SignKey.GetPublic()
		acl := mockSyncAcl{exec.ActualAccounts()["b"].Acl}
		mockCommonSpace.EXPECT().Acl().Return(acl)
		err := fx.ApproveLeave(ctx, spaceId, identity)
		require.Error(t, err)
	})
}
