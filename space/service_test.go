package space

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/anyproto/any-sync/accountservice/mock_accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient/mock_coordinatorclient"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/notifications/mock_notifications"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller/mock_spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/space/spacefactory/mock_spacefactory"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var ctx = context.Background()

const (
	testPersonalSpaceID = "personal.12345"
)

// TODO Revive tests
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
		ctx2, ctxCancel2 := context.WithTimeout(context.Background(), time.Millisecond)
		defer ctxCancel2()

		factory.EXPECT().LoadAndSetTechSpace(ctx2).Return(&clientspace.TechSpace{}, nil)
		require.NoError(t, serv.loadTechSpace(ctx2))

		s, err := serv.Get(ctx2, serv.techSpaceId)
		require.NoError(t, err)
		assert.NotNil(t, s)
	})
	t.Run("existing account", func(t *testing.T) {
		t.Skip("@roman should revive this test")
		fx := newFixture(t, false)
		defer fx.finish(t)
	})
	t.Run("new account", func(t *testing.T) {
		t.Skip("@roman should revive this test")
		fx := newFixture(t, true)
		defer fx.finish(t)
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
		storeFixture.AddObjects(t, storeFixture.TechSpaceId(), []objectstore.TestObject{map[domain.RelationKey]*types.Value{
			bundle.RelationKeyLayout:        pbtypes.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyId:            pbtypes.String("spaceViewId"),
			bundle.RelationKeyTargetSpaceId: pbtypes.String(spaceID),
			bundle.RelationKeyName:          pbtypes.String("Test"),
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

func newFixture(t *testing.T, newAccount bool) *fixture {
	ctrl := gomock.NewController(t)
	fx := &fixture{
		service:        New().(*service),
		a:              new(app.App),
		ctrl:           ctrl,
		spaceCore:      mock_spacecore.NewMockSpaceCoreService(t),
		accountService: mock_accountservice.NewMockService(ctrl),
		coordClient:    mock_coordinatorclient.NewMockCoordinatorClient(ctrl),
		factory:        mock_spacefactory.NewMockSpaceFactory(t),
		isNewAccount:   NewMockisNewAccount(t),
		objectStore:    objectstore.NewStoreFixture(t),
	}
	wallet := mock_wallet.NewMockWallet(t)
	wallet.EXPECT().RepoPath().Return("repo/path")

	fx.a.Register(testutil.PrepareMock(ctx, fx.a, fx.spaceCore)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.coordClient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.accountService)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.isNewAccount)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.factory)).
		Register(testutil.PrepareMock(ctx, fx.a, mock_notifications.NewMockNotifications(t))).
		Register(testutil.PrepareMock(ctx, fx.a, wallet)).
		Register(&config.Config{DisableFileConfig: true, NetworkMode: pb.RpcAccount_LocalOnly, PeferYamuxTransport: true}).
		Register(fx.objectStore).
		Register(fx.service)
	fx.isNewAccount.EXPECT().IsNewAccount().Return(newAccount)
	fx.spaceCore.EXPECT().DeriveID(mock.Anything, mock.Anything).Return(testPersonalSpaceID, nil)
	fx.accountService.EXPECT().Account().Return(&accountdata.AccountKeys{})
	fx.expectRun(t, newAccount)

	require.NoError(t, fx.a.Start(ctx))

	return fx
}

type fixture struct {
	*service
	a              *app.App
	factory        *mock_spacefactory.MockSpaceFactory
	spaceCore      *mock_spacecore.MockSpaceCoreService
	accountService *mock_accountservice.MockService
	coordClient    *mock_coordinatorclient.MockCoordinatorClient
	ctrl           *gomock.Controller
	isNewAccount   *MockisNewAccount
	objectStore    *objectstore.StoreFixture
}

type lwMock struct {
	sp clientspace.Space
}

func (l lwMock) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
	return l.sp, nil
}

func (fx *fixture) expectRun(t *testing.T, newAccount bool) {
	clientSpace := mock_clientspace.NewMockSpace(t)
	mpCtrl := mock_spacecontroller.NewMockSpaceController(t)
	fx.factory.EXPECT().CreateMarketplaceSpace(mock.Anything).Return(mpCtrl, nil)
	mpCtrl.EXPECT().Start(mock.Anything).Return(nil)
	ts := mock_techspace.NewMockTechSpace(t)
	fx.factory.EXPECT().CreateAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: ts}, nil)
	prCtrl := mock_spacecontroller.NewMockSpaceController(t)
	fx.coordClient.EXPECT().StatusCheckMany(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, fmt.Errorf("test not check statuses"))
	if newAccount {
		fx.factory.EXPECT().CreatePersonalSpace(mock.Anything, mock.Anything).Return(prCtrl, nil)
		lw := lwMock{clientSpace}
		prCtrl.EXPECT().Current().Return(lw)
	} else {
		fx.factory.EXPECT().NewPersonalSpace(mock.Anything, mock.Anything).Return(prCtrl, nil)
		lw := lwMock{clientSpace}
		prCtrl.EXPECT().Current().Return(lw)
	}
	prCtrl.EXPECT().Mode().Return(mode.ModeLoading)
	ts.EXPECT().Close(mock.Anything).Return(nil)
	mpCtrl.EXPECT().Close(mock.Anything).Return(nil)
	prCtrl.EXPECT().Close(mock.Anything).Return(nil)
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
