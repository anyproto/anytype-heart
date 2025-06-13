package space

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/mock_commonspace"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/anyproto/any-sync/testutil/accounttest"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/ipfs/go-cid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/notifications/mock_notifications"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/aclobjectmanager"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller/mock_spacecontroller"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/space/spacefactory/mock_spacefactory"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

const (
	testPersonalSpaceID = "personal.12345"
)

type mockAclJoiner struct {
}

func (m *mockAclJoiner) Join(ctx context.Context, spaceId, networkId string, inviteCid cid.Cid, inviteFileKey crypto.SymKey) error {
	return nil
}
func (m *mockAclJoiner) Init(a *app.App) error {
	return nil
}
func (m *mockAclJoiner) Name() string {
	return "aclJoiner"
}

func TestService_Init(t *testing.T) {
	t.Run("tech space getter", func(t *testing.T) {
		serv := New().(*service)
		serv.techSpaceId = "tech.space"
		factory := mock_spacefactory.NewMockSpaceFactory(t)
		serv.factory = factory
		serv.techSpaceReady = make(chan struct{})

		// not initialized - expect context deadline
		ctx, ctxCancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer ctxCancel()
		_, err := serv.Get(ctx, serv.techSpaceId)
		require.ErrorIs(t, err, context.DeadlineExceeded)

		// initialized - expect space
		ctx2, ctxCancel2 := context.WithTimeout(context.Background(), 2*time.Millisecond)
		defer ctxCancel2()

		factory.EXPECT().LoadAndSetTechSpace(ctx2).Return(&clientspace.TechSpace{}, nil)
		require.NoError(t, serv.loadTechSpace(ctx2))

		s, err := serv.Get(ctx2, serv.techSpaceId)
		require.NoError(t, err)
		assert.NotNil(t, s)
	})
	t.Run("new account", func(t *testing.T) {
		newFixture(t, nil)
	})
	t.Run("old account", func(t *testing.T) {
		newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().WakeUpViews()
		})
	})
	t.Run("old account, no internet, then internet appeared", func(t *testing.T) {
		newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(nil, context.DeadlineExceeded).Times(1)
			fx.spaceCore.EXPECT().StorageExistsLocally(mock.Anything, fx.spaceId).Return(false, nil)
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().WakeUpViews()
		})
	})
	t.Run("old account, no internet, but personal space exists", func(t *testing.T) {
		newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(nil, context.DeadlineExceeded).Times(1)
			fx.spaceCore.EXPECT().StorageExistsLocally(mock.Anything, fx.spaceId).Return(true, nil)
			fx.spaceCore.EXPECT().Get(mock.Anything, fx.spaceId).Return(nil, nil)
			fx.factory.EXPECT().CreateAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			prCtrl := mock_spacecontroller.NewMockSpaceController(t)
			fx.factory.EXPECT().NewPersonalSpace(mock.Anything, mock.Anything).Return(prCtrl, nil)
			prCtrl.EXPECT().Close(mock.Anything).Return(nil)
			fx.techSpace.EXPECT().WakeUpViews()
		})
	})
	t.Run("very old account without tech space", func(t *testing.T) {
		newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(nil, spacesyncproto.ErrSpaceMissing)
			fx.spaceCore.EXPECT().Get(mock.Anything, fx.spaceId).Return(nil, nil)
			fx.factory.EXPECT().CreateAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			prCtrl := mock_spacecontroller.NewMockSpaceController(t)
			fx.factory.EXPECT().NewPersonalSpace(mock.Anything, mock.Anything).Return(prCtrl, nil)
			prCtrl.EXPECT().Close(mock.Anything).Return(nil)
			fx.techSpace.EXPECT().WakeUpViews()
		})
	})
}

func TestService_UpdateRemoteStatus(t *testing.T) {
	spaceID := "id"
	t.Run("don't send notification, because account status deleted", func(t *testing.T) {
		// given
		controller := mock_spacecontroller.NewMockSpaceController(t)
		statusInfo := makeRemoteInfo(spaceID, false, spaceinfo.RemoteStatusDeleted)
		controller.EXPECT().SetLocalInfo(context.Background(), statusInfo.LocalInfo).Return(nil)
		controller.EXPECT().GetStatus().Return(spaceinfo.AccountStatusDeleted)
		notifications := mock_notifications.NewMockNotifications(t)
		s := service{
			spaceControllers:    map[string]spacecontroller.SpaceController{spaceID: controller},
			notificationService: notifications,
		}

		// when
		err := s.UpdateRemoteStatus(ctx, statusInfo)

		// then
		notifications.AssertNotCalled(t, "CreateAndSend")
		assert.Nil(t, err)
	})
	t.Run("don't send notification, because account status removing", func(t *testing.T) {
		// given
		controller := mock_spacecontroller.NewMockSpaceController(t)
		statusInfo := makeRemoteInfo(spaceID, false, spaceinfo.RemoteStatusDeleted)
		controller.EXPECT().SetLocalInfo(context.Background(), statusInfo.LocalInfo).Return(nil)
		controller.EXPECT().GetStatus().Return(spaceinfo.AccountStatusRemoving)
		notifications := mock_notifications.NewMockNotifications(t)
		s := service{
			spaceControllers:    map[string]spacecontroller.SpaceController{spaceID: controller},
			notificationService: notifications,
		}

		// when
		err := s.UpdateRemoteStatus(ctx, statusInfo)

		// then
		notifications.AssertNotCalled(t, "CreateAndSend")
		assert.Nil(t, err)
	})
	t.Run("don't send notification, because space status - not deleted", func(t *testing.T) {
		// given
		controller := mock_spacecontroller.NewMockSpaceController(t)
		statusInfo := makeRemoteInfo(spaceID, false, spaceinfo.RemoteStatusOk)
		controller.EXPECT().SetLocalInfo(context.Background(), statusInfo.LocalInfo).Return(nil)
		notifications := mock_notifications.NewMockNotifications(t)
		s := service{
			spaceControllers:    map[string]spacecontroller.SpaceController{spaceID: controller},
			notificationService: notifications,
		}

		// when
		err := s.UpdateRemoteStatus(ctx, statusInfo)

		// then
		notifications.AssertNotCalled(t, "CreateAndSend")
		assert.Nil(t, err)
	})
	t.Run("send notification, because space status - deleted, but we can't get space name", func(t *testing.T) {
		// given
		controller := mock_spacecontroller.NewMockSpaceController(t)
		statusInfo := makeRemoteInfo(spaceID, false, spaceinfo.RemoteStatusDeleted)
		controller.EXPECT().SetLocalInfo(context.Background(), statusInfo.LocalInfo).Return(nil)
		controller.EXPECT().GetStatus().Return(spaceinfo.AccountStatusActive)
		controller.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		controller.EXPECT().SetPersistentInfo(context.Background(), makePersistentInfo(spaceID, spaceinfo.AccountStatusRemoving)).Return(nil)

		accountKeys, err := accountdata.NewRandom()
		assert.Nil(t, err)
		wallet := mock_wallet.NewMockWallet(t)
		wallet.EXPECT().Account().Return(accountKeys)
		identity := accountKeys.SignKey.GetPublic().Account()

		notifications := mock_notifications.NewMockNotifications(t)
		notifications.EXPECT().CreateAndSend(&model.Notification{
			Id: strings.Join([]string{spaceID, identity}, "_"),
			Payload: &model.NotificationPayloadOfParticipantRemove{
				ParticipantRemove: &model.NotificationParticipantRemove{
					SpaceId:   spaceID,
					SpaceName: "",
				},
			},
			Space: spaceID,
		}).Return(nil)

		storeFixture := objectstore.NewStoreFixture(t)
		s := service{
			spaceControllers:    map[string]spacecontroller.SpaceController{spaceID: controller},
			notificationService: notifications,
			accountService:      wallet,
			spaceNameGetter:     storeFixture,
		}

		// when
		err = s.UpdateRemoteStatus(ctx, statusInfo)

		// then
		assert.Nil(t, err)
	})
	t.Run("send notification, because space remote status - deleted, but we get space name with name Test", func(t *testing.T) {
		// given
		controller := mock_spacecontroller.NewMockSpaceController(t)
		statusInfo := makeRemoteInfo(spaceID, false, spaceinfo.RemoteStatusDeleted)
		controller.EXPECT().SetLocalInfo(context.Background(), statusInfo.LocalInfo).Return(nil)
		controller.EXPECT().GetStatus().Return(spaceinfo.AccountStatusActive)
		controller.EXPECT().GetLocalStatus().Return(spaceinfo.LocalStatusOk)
		controller.EXPECT().SetPersistentInfo(context.Background(), makePersistentInfo(spaceID, spaceinfo.AccountStatusRemoving)).Return(nil)

		accountKeys, err := accountdata.NewRandom()
		assert.Nil(t, err)
		wallet := mock_wallet.NewMockWallet(t)
		wallet.EXPECT().Account().Return(accountKeys)
		identity := accountKeys.SignKey.GetPublic().Account()

		notifications := mock_notifications.NewMockNotifications(t)
		notifications.EXPECT().CreateAndSend(&model.Notification{
			Id: strings.Join([]string{spaceID, identity}, "_"),
			Payload: &model.NotificationPayloadOfParticipantRemove{
				ParticipantRemove: &model.NotificationParticipantRemove{
					SpaceId:   spaceID,
					SpaceName: "Test",
				},
			},
			Space: spaceID,
		}).Return(nil)

		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, storeFixture.TechSpaceId(), []objectstore.TestObject{{
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyId:             domain.String("spaceViewId"),
			bundle.RelationKeyTargetSpaceId:  domain.String(spaceID),
			bundle.RelationKeyName:           domain.String("Test"),
		}})

		s := service{
			spaceControllers:    map[string]spacecontroller.SpaceController{spaceID: controller},
			notificationService: notifications,
			accountService:      wallet,
			spaceNameGetter:     storeFixture,
		}

		// when
		err = s.UpdateRemoteStatus(ctx, statusInfo)

		// then
		assert.Nil(t, err)
	})
}

func TestService_UpdateSharedLimits(t *testing.T) {
	t.Run("update shared limits", func(t *testing.T) {
		// given
		mockTechSpace := mock_techspace.NewMockTechSpace(t)
		s := service{
			personalSpaceId: "spaceId",
			techSpace:       &clientspace.TechSpace{TechSpace: mockTechSpace},
		}
		mockAccountObject := mock_techspace.NewMockAccountObject(t)
		mockTechSpace.EXPECT().DoAccountObject(ctx, mock.Anything).RunAndReturn(
			func(ctx context.Context, f func(view techspace.AccountObject) error) error {
				return f(mockAccountObject)
			})
		mockAccountObject.EXPECT().SetSharedSpacesLimit(10).Return(nil)

		// when
		err := s.UpdateSharedLimits(ctx, 10)

		// then
		require.NoError(t, err)
	})
}

func newFixture(t *testing.T, expectOldAccount func(t *testing.T, fx *fixture)) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		spaceId:            "bafyreifhyhdwrhwc23yi52w42osr4erqhiu2domqd3vwnngdee23kulpre.3aop5yrnf383q",
		service:            New().(*service),
		a:                  new(app.App),
		ctrl:               ctrl,
		spaceCore:          mock_spacecore.NewMockSpaceCoreService(t),
		coordClient:        mock_coordinatorclient.NewMockCoordinatorClient(ctrl),
		factory:            mock_spacefactory.NewMockSpaceFactory(t),
		notificationSender: mock_space.NewMockNotificationSender(t),
		objectStore:        objectstore.NewStoreFixture(t),
		updater:            mock_space.NewMockcoordinatorStatusUpdater(t),
		config:             config.New(config.WithNewAccount(expectOldAccount == nil)),
	}
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	fx.config.PeferYamuxTransport = true
	wallet := mock_wallet.NewMockWallet(t)
	path, err := os.MkdirTemp("", "repo")
	require.NoError(t, err)
	defer os.RemoveAll(path)
	wallet.EXPECT().Account().Return(keys)
	wallet.EXPECT().RepoPath().Return(path)

	fx.a.
		Register(testutil.PrepareMock(ctx, fx.a, wallet)).
		Register(fx.config).
		Register(&mockAclJoiner{}).
		Register(testutil.PrepareMock(ctx, fx.a, fx.notificationSender)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.updater)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.spaceCore)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.coordClient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.factory)).
		Register(testutil.PrepareMock(ctx, fx.a, mock_notifications.NewMockNotifications(t))).
		Register(fx.objectStore).
		Register(&testSpaceLoaderListener{}).
		Register(fx.service)
	fx.expectRun(t, expectOldAccount)

	require.NoError(t, fx.a.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, fx.a.Close(ctx))
	})
	return fx
}

type fixture struct {
	*service
	spaceId            string
	a                  *app.App
	config             *config.Config
	factory            *mock_spacefactory.MockSpaceFactory
	spaceCore          *mock_spacecore.MockSpaceCoreService
	updater            *mock_space.MockcoordinatorStatusUpdater
	notificationSender *mock_space.MockNotificationSender
	accountService     *accounttest.AccountTestService
	coordClient        *mock_coordinatorclient.MockCoordinatorClient
	ctrl               *gomock.Controller
	techSpace          *mock_techspace.MockTechSpace
	clientSpace        *mock_clientspace.MockSpace
	objectStore        *objectstore.StoreFixture
}

type lwMock struct {
	sp clientspace.Space
}

func (l lwMock) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
	return l.sp, nil
}

func (fx *fixture) expectRun(t *testing.T, expectOldAccount func(t *testing.T, fx *fixture)) {
	fx.spaceCore.EXPECT().DeriveID(mock.Anything, spacecore.SpaceType).Return(fx.spaceId, nil).Times(1)
	fx.spaceCore.EXPECT().DeriveID(mock.Anything, spacecore.TechSpaceType).Return("techSpaceId", nil).Times(1)
	fx.updater.EXPECT().UpdateCoordinatorStatus()
	clientSpace := mock_clientspace.NewMockSpace(t)
	mpCtrl := mock_spacecontroller.NewMockSpaceController(t)
	fx.factory.EXPECT().CreateMarketplaceSpace(mock.Anything).Return(mpCtrl, nil)
	mpCtrl.EXPECT().Start(mock.Anything).Return(nil)
	mpCtrl.EXPECT().Close(mock.Anything).Return(nil)
	ts := mock_techspace.NewMockTechSpace(t)
	fx.techSpace = ts
	fx.clientSpace = clientSpace
	if expectOldAccount == nil {
		fx.factory.EXPECT().CreateAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: ts}, nil)
		prCtrl := mock_spacecontroller.NewMockSpaceController(t)
		prCtrl.EXPECT().SpaceId().Return(fx.spaceId)
		commonSpace := mock_commonspace.NewMockSpace(fx.ctrl)
		commonSpace.EXPECT().Id().Return(fx.spaceId).AnyTimes()
		fx.spaceCore.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).Return(&spacecore.AnySpace{Space: commonSpace}, nil)
		fx.factory.EXPECT().CreateShareableSpace(mock.Anything, mock.Anything, mock.Anything).Return(prCtrl, nil)
		lw := lwMock{clientSpace}
		clientSpace.EXPECT().Id().Return(fx.spaceId)
		prCtrl.EXPECT().Current().Return(lw)
		prCtrl.EXPECT().Close(mock.Anything).Return(nil)
		ts.EXPECT().WakeUpViews()
	} else {
		expectOldAccount(t, fx)
	}
	return
}

func (fx *fixture) finish(t *testing.T) {
	require.NoError(t, fx.a.Close(ctx))
	fx.ctrl.Finish()
}

func makePersistentInfo(spaceId string, status spaceinfo.AccountStatus) spaceinfo.SpacePersistentInfo {
	info := spaceinfo.NewSpacePersistentInfo(spaceId)
	info.SetAccountStatus(status)
	return info
}

func makeRemoteInfo(spaceId string, isOwned bool, status spaceinfo.RemoteStatus) spaceinfo.SpaceRemoteStatusInfo {
	info := spaceinfo.SpaceRemoteStatusInfo{
		IsOwned:   isOwned,
		LocalInfo: makeLocalInfo(spaceId, status),
	}
	return info
}

func makeLocalInfo(spaceId string, remoteStatus spaceinfo.RemoteStatus) spaceinfo.SpaceLocalInfo {
	info := spaceinfo.NewSpaceLocalInfo(spaceId)
	info.SetRemoteStatus(remoteStatus)
	return info
}

type testSpaceLoaderListener struct {
	aclobjectmanager.SpaceLoaderListener
	app.Component
}

func (s *testSpaceLoaderListener) OnSpaceLoad(_ string)   {}
func (s *testSpaceLoaderListener) OnSpaceUnload(_ string) {}

func (s *testSpaceLoaderListener) Init(a *app.App) (err error) { return nil }
func (s *testSpaceLoaderListener) Name() (name string)         { return "spaceLoaderListener" }
