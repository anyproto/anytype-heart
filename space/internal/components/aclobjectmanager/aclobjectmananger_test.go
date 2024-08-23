package aclobjectmanager

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl"
	"github.com/anyproto/any-sync/commonspace/object/acl/syncacl/headupdater"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/aclnotifications/mock_aclnotifications"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies/mock_dependencies"
	"github.com/anyproto/anytype-heart/space/internal/components/invitemigrator/mock_invitemigrator"
	"github.com/anyproto/anytype-heart/space/internal/components/participantwatcher/mock_participantwatcher"
	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader/mock_spaceloader"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus/mock_spacestatus"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

func TestAclObjectManager(t *testing.T) {
	t.Run("owner", func(t *testing.T) {
		a := list.NewAclExecutor("spaceId")
		cmds := []string{
			"a.init::a",
		}
		for _, cmd := range cmds {
			err := a.Execute(cmd)
			require.NoError(t, err)
		}
		acl := &syncAclStub{AclList: a.ActualAccounts()["a"].Acl}
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
		fx.mockInviteMigrator.EXPECT().MigrateExistingInvites(fx.mockSpace).Return(nil)
		fx.mockParticipantWatcher.EXPECT().UpdateAccountParticipantFromProfile(mock.Anything, fx.mockSpace).Return(nil)
		fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
		fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("")
		fx.mockParticipantWatcher.EXPECT().UpdateParticipantFromAclState(mock.Anything, fx.mockSpace, mock.Anything).
			RunAndReturn(func(_ context.Context, space clientspace.Space, state list.AccountState) error {
				require.True(t, state.PubKey.Equals(acl.AclState().Identity()))
				return nil
			})
		fx.mockParticipantWatcher.EXPECT().WatchParticipant(mock.Anything, fx.mockSpace, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetAclIsEmpty(true).Return(nil)
		fx.mockCommonSpace.EXPECT().Id().Return("spaceId")
		fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsOwner, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)
		fx.run(t)
		<-fx.aclObjectManager.wait
		fx.aclObjectManager.mx.Lock()
		defer fx.aclObjectManager.mx.Unlock()
		require.Equal(t, acl.Head().Id, fx.aclObjectManager.lastIndexed)
		require.Equal(t, fx.aclObjectManager, acl.updater)
	})
	t.Run("participant", func(t *testing.T) {
		a := list.NewAclExecutor("spaceId")
		cmds := []string{
			"a.init::a",
			"a.invite::invId",
			"b.join::invId",
			"a.approve::b,r",
		}
		for _, cmd := range cmds {
			err := a.Execute(cmd)
			require.NoError(t, err)
		}
		acl := &syncAclStub{AclList: a.ActualAccounts()["b"].Acl}
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
		fx.mockInviteMigrator.EXPECT().MigrateExistingInvites(fx.mockSpace).Return(nil)
		fx.mockParticipantWatcher.EXPECT().UpdateAccountParticipantFromProfile(mock.Anything, fx.mockSpace).Return(nil)
		fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
		fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("")
		var callCounter atomic.Bool
		fx.mockParticipantWatcher.EXPECT().UpdateParticipantFromAclState(mock.Anything, fx.mockSpace, mock.Anything).
			RunAndReturn(func(_ context.Context, space clientspace.Space, state list.AccountState) error {
				if !callCounter.Load() {
					require.True(t, state.PubKey.Equals(a.ActualAccounts()["a"].Keys.SignKey.GetPublic()))
					callCounter.Store(true)
				} else {
					require.True(t, state.PubKey.Equals(acl.AclState().Identity()))
				}
				return nil
			})
		fx.mockParticipantWatcher.EXPECT().WatchParticipant(mock.Anything, fx.mockSpace, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetAclIsEmpty(false).Return(nil)
		fx.mockCommonSpace.EXPECT().Id().Return("spaceId")
		fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsReader, "spaceId", spaceinfo.AccountStatusActive, spaceinfo.LocalStatusOk)
		fx.run(t)
		<-fx.aclObjectManager.wait
		fx.aclObjectManager.mx.Lock()
		defer fx.aclObjectManager.mx.Unlock()
		require.Equal(t, acl.Head().Id, fx.aclObjectManager.lastIndexed)
		require.Equal(t, fx.aclObjectManager, acl.updater)
	})
	t.Run("participant removed", func(t *testing.T) {
		a := list.NewAclExecutor("spaceId")
		cmds := []string{
			"a.init::a",
			"a.invite::invId",
			"b.join::invId",
			"a.approve::b,r",
			"a.remove::b",
		}
		for _, cmd := range cmds {
			err := a.Execute(cmd)
			require.NoError(t, err)
		}
		acl := &syncAclStub{AclList: a.ActualAccounts()["b"].Acl}
		fx := newFixture(t)
		defer fx.finish(t)
		fx.mockLoader.EXPECT().WaitLoad(mock.Anything).Return(fx.mockSpace, nil)
		fx.mockInviteMigrator.EXPECT().MigrateExistingInvites(fx.mockSpace).Return(nil)
		fx.mockParticipantWatcher.EXPECT().UpdateAccountParticipantFromProfile(mock.Anything, fx.mockSpace).Return(nil)
		fx.mockSpace.EXPECT().CommonSpace().Return(fx.mockCommonSpace)
		fx.mockCommonSpace.EXPECT().Acl().AnyTimes().Return(acl)
		fx.mockStatus.EXPECT().GetLatestAclHeadId().Return("")
		fx.mockParticipantWatcher.EXPECT().UpdateParticipantFromAclState(mock.Anything, fx.mockSpace, mock.Anything).
			RunAndReturn(func(_ context.Context, space clientspace.Space, state list.AccountState) error {
				require.True(t, state.PubKey.Equals(a.ActualAccounts()["a"].Keys.SignKey.GetPublic()))
				return nil
			})
		fx.mockParticipantWatcher.EXPECT().WatchParticipant(mock.Anything, fx.mockSpace, mock.Anything).Return(nil)
		fx.mockStatus.EXPECT().SetPersistentStatus(spaceinfo.AccountStatusRemoving).Return(nil)
		fx.mockStatus.EXPECT().SetAclIsEmpty(false).Return(nil)
		fx.mockCommonSpace.EXPECT().Id().Return("spaceId")
		fx.mockStatus.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		fx.mockAclNotification.EXPECT().AddRecords(acl, list.AclPermissionsNone, "spaceId", spaceinfo.AccountStatusDeleted, spaceinfo.LocalStatusOk)
		fx.run(t)
		<-fx.aclObjectManager.wait
		fx.aclObjectManager.mx.Lock()
		defer fx.aclObjectManager.mx.Unlock()
		require.Equal(t, acl.Head().Id, fx.aclObjectManager.lastIndexed)
		require.Equal(t, fx.aclObjectManager, acl.updater)
	})
}

var ctx = context.Background()

type fixture struct {
	*aclObjectManager
	a                      *app.App
	ctrl                   *gomock.Controller
	mockStatus             *mock_spacestatus.MockSpaceStatus
	mockIndexer            *mock_dependencies.MockSpaceIndexer
	mockLoader             *mock_spaceloader.MockSpaceLoader
	mockSpace              *mock_clientspace.MockSpace
	mockCommonSpace        *mock_commonspace.MockSpace
	mockParticipantWatcher *mock_participantwatcher.MockParticipantWatcher
	mockAclNotification    *mock_aclnotifications.MockAclNotification
	mockInviteMigrator     *mock_invitemigrator.MockInviteMigrator
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		aclObjectManager:       New(nil).(*aclObjectManager),
		ctrl:                   ctrl,
		a:                      new(app.App),
		mockStatus:             mock_spacestatus.NewMockSpaceStatus(t),
		mockIndexer:            mock_dependencies.NewMockSpaceIndexer(t),
		mockLoader:             mock_spaceloader.NewMockSpaceLoader(t),
		mockSpace:              mock_clientspace.NewMockSpace(t),
		mockCommonSpace:        mock_commonspace.NewMockSpace(ctrl),
		mockParticipantWatcher: mock_participantwatcher.NewMockParticipantWatcher(t),
		mockAclNotification:    mock_aclnotifications.NewMockAclNotification(t),
		mockInviteMigrator:     mock_invitemigrator.NewMockInviteMigrator(t),
	}
	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.mockStatus)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockInviteMigrator)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockIndexer)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockLoader)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockParticipantWatcher)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.mockAclNotification)).
		Register(fx)
	return fx
}

func (fx *fixture) run(t *testing.T) {
	require.NoError(t, fx.a.Start(ctx))
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}

type syncAclStub struct {
	list.AclList
	updater headupdater.AclUpdater
}

func (s *syncAclStub) HandleMessage(ctx context.Context, senderId string, protoVersion uint32, message *spacesyncproto.ObjectSyncMessage) (err error) {
	return
}

func (s *syncAclStub) SyncWithPeer(ctx context.Context, p peer.Peer) (err error) {
	return
}

func (s *syncAclStub) Init(a *app.App) (err error) {
	return nil
}

func (s *syncAclStub) Name() (name string) {
	return syncacl.CName
}

func (s *syncAclStub) Run(ctx context.Context) (err error) {
	return
}

func (s *syncAclStub) HandleRequest(ctx context.Context, senderId string, request *spacesyncproto.ObjectSyncMessage) (response *spacesyncproto.ObjectSyncMessage, err error) {
	return
}

func (s *syncAclStub) SetHeadUpdater(updater headupdater.HeadUpdater) {
	return
}

func (s *syncAclStub) SetAclUpdater(updater headupdater.AclUpdater) {
	s.updater = updater
	return
}
