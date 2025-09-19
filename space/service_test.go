package space

import (
	"context"
	"fmt"
	"os"
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

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban/mock_kanban"
	"github.com/anyproto/anytype-heart/core/notifications/mock_notifications"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/aclobjectmanager"
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
			fx.techSpace.EXPECT().StartSync()
		})
	})
	t.Run("old account, no internet, then internet appeared", func(t *testing.T) {
		newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(nil, context.DeadlineExceeded).Times(1)
			fx.spaceCore.EXPECT().StorageExistsLocally(mock.Anything, fx.spaceId).Return(false, nil)
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().StartSync()
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
			fx.techSpace.EXPECT().StartSync()
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
			fx.techSpace.EXPECT().StartSync()
		})
	})
}

func TestService_UpdateRemoteStatus(t *testing.T) {
	spaceID := "id"
	t.Run("successfully updates remote status", func(t *testing.T) {
		// given
		statusInfo := makeRemoteInfo(spaceID, false, spaceinfo.RemoteStatusDeleted)

		mockTechSpace := mock_techspace.NewMockTechSpace(t)
		mockSpaceView := mock_techspace.NewMockSpaceView(t)
		mockSpaceView.EXPECT().SetSpaceLocalInfo(statusInfo.LocalInfo).Return(nil)
		mockTechSpace.EXPECT().DoSpaceView(ctx, statusInfo.LocalInfo.SpaceId, mock.Anything).RunAndReturn(
			func(ctx context.Context, spaceID string, apply func(spaceView techspace.SpaceView) error) error {
				return apply(mockSpaceView)
			})

		s := service{
			techSpace: &clientspace.TechSpace{TechSpace: mockTechSpace},
		}

		// when
		err := s.UpdateRemoteStatus(ctx, statusInfo)

		// then
		assert.NoError(t, err)
	})
	t.Run("returns error when DoSpaceView fails", func(t *testing.T) {
		// given
		statusInfo := makeRemoteInfo(spaceID, false, spaceinfo.RemoteStatusDeleted)
		expectedErr := fmt.Errorf("space view error")

		mockTechSpace := mock_techspace.NewMockTechSpace(t)
		mockTechSpace.EXPECT().DoSpaceView(ctx, statusInfo.LocalInfo.SpaceId, mock.Anything).Return(expectedErr)

		s := service{
			techSpace: &clientspace.TechSpace{TechSpace: mockTechSpace},
		}

		// when
		err := s.UpdateRemoteStatus(ctx, statusInfo)

		// then
		assert.Equal(t, expectedErr, err)
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

	eventQueue := mb.New[*pb.EventMessage](0)
	kanbanService := mock_kanban.NewMockService(t)
	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Run(func(e *pb.Event) {
		for _, msg := range e.Messages {
			eventQueue.Add(context.Background(), msg)
		}
	}).Maybe()
	collService := &dummyCollectionService{}
	subscriptionService := subscription.New()
	fx.a.
		Register(testutil.PrepareMock(ctx, fx.a, wallet)).
		Register(fx.config).
		Register(&mockAclJoiner{}).
		Register(testutil.PrepareMock(ctx, fx.a, kanbanService)).
		Register(testutil.PrepareMock(ctx, fx.a, eventSender)).
		Register(fx.objectStore).
		Register(collService).
		Register(subscriptionService).
		Register(testutil.PrepareMock(ctx, fx.a, fx.notificationSender)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.updater)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.spaceCore)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.coordClient)).
		Register(testutil.PrepareMock(ctx, fx.a, fx.factory)).
		Register(testutil.PrepareMock(ctx, fx.a, mock_notifications.NewMockNotifications(t))).
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
		fx.spaceCore.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(&spacecore.AnySpace{Space: commonSpace}, nil)
		fx.factory.EXPECT().CreateShareableSpace(mock.Anything, mock.Anything, mock.Anything).Return(prCtrl, nil)
		lw := lwMock{clientSpace}
		clientSpace.EXPECT().Id().Return(fx.spaceId)
		prCtrl.EXPECT().Current().Return(lw)
		prCtrl.EXPECT().Close(mock.Anything).Return(nil)
		ts.EXPECT().StartSync()
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

type dummyCollectionService struct{}

func (d *dummyCollectionService) Init(a *app.App) (err error) {
	return nil
}

func (d *dummyCollectionService) Name() (name string) {
	return "dummyCollectionService"
}

func (d *dummyCollectionService) SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error) {
	return nil, nil, nil
}

func (d *dummyCollectionService) UnsubscribeFromCollection(collectionID string, subscriptionID string) error {
	return nil
}

func givenSpaceViewObject(id string, targetSpaceId string, creator string, accountStatus spaceinfo.AccountStatus, remoteStatus spaceinfo.RemoteStatus, localStatus spaceinfo.LocalStatus, guestKey string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                 domain.String(id),
		bundle.RelationKeyTargetSpaceId:      domain.String(targetSpaceId),
		bundle.RelationKeyCreator:            domain.String(creator),
		bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(accountStatus)),
		bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(localStatus)),
		bundle.RelationKeySpaceRemoteStatus:  domain.Int64(int64(remoteStatus)),
		bundle.RelationKeyGuestKey:           domain.String(guestKey),
	}
}

func TestService_onSpaceStatusUpdated(t *testing.T) {
	t.Run("personal space - creates personal space controller and calls update", func(t *testing.T) {
		fx := newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().StartSync()
		})

		done := make(chan struct{})
		personalCtrl := mock_spacecontroller.NewMockSpaceController(t)
		personalCtrl.EXPECT().SpaceId().Return(fx.spaceId).Maybe()
		personalCtrl.EXPECT().Close(mock.Anything).Return(nil).Maybe()
		fx.factory.EXPECT().NewPersonalSpace(mock.Anything, fx.service.accountMetadataPayload).Return(personalCtrl, nil)
		personalCtrl.EXPECT().Update().Run(func() {
			close(done)
		}).Return(nil).Once()

		fx.objectStore.AddObjects(t, fx.service.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView1", fx.spaceId, "creator1", spaceinfo.AccountStatusActive, spaceinfo.RemoteStatusOk, spaceinfo.LocalStatusOk, ""),
		})

		<-done
	})

	t.Run("shareable space - creates shareable space controller and calls update", func(t *testing.T) {
		fx := newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().StartSync()
		})

		shareableSpaceId := "shareable.space.id"
		done := make(chan struct{})
		shareableCtrl := mock_spacecontroller.NewMockSpaceController(t)
		shareableCtrl.EXPECT().SpaceId().Return(shareableSpaceId).Maybe()
		shareableCtrl.EXPECT().Close(mock.Anything).Return(nil).Maybe()
		fx.factory.EXPECT().NewShareableSpace(mock.Anything, shareableSpaceId, mock.Anything).Return(shareableCtrl, nil)
		shareableCtrl.EXPECT().Update().Run(func() {
			close(done)
		}).Return(nil).Once()

		fx.objectStore.AddObjects(t, fx.service.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView2", shareableSpaceId, "creator2", spaceinfo.AccountStatusActive, spaceinfo.RemoteStatusOk, spaceinfo.LocalStatusOk, ""),
		})

		<-done
	})

	t.Run("streamable space - creates streamable space controller and calls update", func(t *testing.T) {
		fx := newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().StartSync()
		})

		streamableSpaceId := "streamable.space.id"
		done := make(chan struct{})
		streamableCtrl := mock_spacecontroller.NewMockSpaceController(t)
		streamableCtrl.EXPECT().SpaceId().Return(streamableSpaceId).Maybe()
		streamableCtrl.EXPECT().Close(mock.Anything).Return(nil).Maybe()
		fx.factory.EXPECT().NewStreamableSpace(mock.Anything, streamableSpaceId, mock.Anything, fx.service.accountMetadataPayload).Return(streamableCtrl, nil)
		streamableCtrl.EXPECT().Update().Run(func() {
			close(done)
		}).Return(nil).Once()

		fx.objectStore.AddObjects(t, fx.service.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView3", streamableSpaceId, "creator3", spaceinfo.AccountStatusActive, spaceinfo.RemoteStatusOk, spaceinfo.LocalStatusOk, "encoded_key_here"),
		})

		<-done
	})

	t.Run("space deleted remotely - sets account status to deleted and sends notification", func(t *testing.T) {
		fx := newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().StartSync()
		})

		deletedSpaceId := "deleted.space.id"
		done := make(chan struct{})
		mockSpaceView := mock_techspace.NewMockSpaceView(t)
		fx.techSpace.EXPECT().DoSpaceView(mock.Anything, deletedSpaceId, mock.Anything).RunAndReturn(
			func(ctx context.Context, spaceID string, apply func(spaceView techspace.SpaceView) error) error {
				return apply(mockSpaceView)
			})
		mockSpaceView.EXPECT().SetSpacePersistentInfo(mock.MatchedBy(func(info spaceinfo.SpacePersistentInfo) bool {
			return info.SpaceID == deletedSpaceId && info.GetAccountStatus() == spaceinfo.AccountStatusDeleted
		})).Run(func(info spaceinfo.SpacePersistentInfo) {
			close(done)
		}).Return(nil)

		fx.notificationSender.EXPECT().CreateAndSend(mock.Anything).Return(nil)

		fx.objectStore.AddObjects(t, fx.service.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView4", deletedSpaceId, "creator4", spaceinfo.AccountStatusActive, spaceinfo.RemoteStatusDeleted, spaceinfo.LocalStatusOk, ""),
		})

		<-done
	})

	t.Run("space already deleted - does not process deletion again", func(t *testing.T) {
		fx := newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().StartSync()
		})

		alreadyDeletedSpaceId := "already.deleted.space"
		done := make(chan struct{})
		shareableCtrl := mock_spacecontroller.NewMockSpaceController(t)
		shareableCtrl.EXPECT().SpaceId().Return(alreadyDeletedSpaceId).Maybe()
		shareableCtrl.EXPECT().Close(mock.Anything).Return(nil).Maybe()
		fx.factory.EXPECT().NewShareableSpace(mock.Anything, alreadyDeletedSpaceId, mock.Anything).Return(shareableCtrl, nil)
		shareableCtrl.EXPECT().Update().Run(func() {
			close(done)
		}).Return(nil).Once()

		fx.objectStore.AddObjects(t, fx.service.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView5", alreadyDeletedSpaceId, "creator5", spaceinfo.AccountStatusDeleted, spaceinfo.RemoteStatusDeleted, spaceinfo.LocalStatusOk, ""),
		})

		<-done
	})

	t.Run("handles startStatus error gracefully", func(t *testing.T) {
		fx := newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().StartSync()
		})

		errorSpaceId := "error.space.id"
		fx.factory.EXPECT().NewShareableSpace(mock.Anything, errorSpaceId, mock.Anything).Return(nil, fmt.Errorf("factory error"))

		fx.objectStore.AddObjects(t, fx.service.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView6", errorSpaceId, "creator6", spaceinfo.AccountStatusActive, spaceinfo.RemoteStatusOk, spaceinfo.LocalStatusOk, ""),
		})

		time.Sleep(200 * time.Millisecond)
	})

	t.Run("handles controller update error gracefully", func(t *testing.T) {
		fx := newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().StartSync()
		})

		updateErrorSpaceId := "update.error.space.id"
		updateErrorCtrl := mock_spacecontroller.NewMockSpaceController(t)
		updateErrorCtrl.EXPECT().SpaceId().Return(updateErrorSpaceId).Maybe()
		updateErrorCtrl.EXPECT().Close(mock.Anything).Return(nil).Maybe()
		fx.factory.EXPECT().NewShareableSpace(mock.Anything, updateErrorSpaceId, mock.Anything).Return(updateErrorCtrl, nil)
		updateErrorCtrl.EXPECT().Update().Return(fmt.Errorf("update error"))

		fx.objectStore.AddObjects(t, fx.service.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView7", updateErrorSpaceId, "creator7", spaceinfo.AccountStatusActive, spaceinfo.RemoteStatusOk, spaceinfo.LocalStatusOk, ""),
		})

		time.Sleep(200 * time.Millisecond)
	})

	t.Run("service closing - returns early", func(t *testing.T) {
		fx := newFixture(t, func(t *testing.T, fx *fixture) {
			fx.factory.EXPECT().LoadAndSetTechSpace(mock.Anything).Return(&clientspace.TechSpace{TechSpace: fx.techSpace}, nil)
			fx.techSpace.EXPECT().StartSync()
		})

		fx.service.isClosing.Store(true)

		fx.objectStore.AddObjects(t, fx.service.techSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("spaceView8", "closing.space.id", "creator8", spaceinfo.AccountStatusActive, spaceinfo.RemoteStatusOk, spaceinfo.LocalStatusOk, ""),
		})

		time.Sleep(200 * time.Millisecond)
	})
}
