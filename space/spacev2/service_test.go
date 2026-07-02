package spacev2

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban/mock_kanban"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/space/spacedomain"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

var ctx = context.Background()

const (
	testPersonalSpaceId = "personal.12345"
	testTechSpaceId     = "tech.12345"
)

type testSpaceLoaderListener struct{}

func (s *testSpaceLoaderListener) OnSpaceLoad(_ string)        {}
func (s *testSpaceLoaderListener) OnSpaceUnload(_ string)      {}
func (s *testSpaceLoaderListener) Init(a *app.App) (err error) { return nil }
func (s *testSpaceLoaderListener) Name() (name string)         { return "spaceLoaderListener" }

type dummyCollectionService struct{}

func (d *dummyCollectionService) Init(a *app.App) (err error) { return nil }
func (d *dummyCollectionService) Name() (name string)         { return "dummyCollectionService" }

func (d *dummyCollectionService) SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error) {
	return nil, nil, nil
}

func (d *dummyCollectionService) UnsubscribeFromCollection(collectionID string, subscriptionID string) error {
	return nil
}

type fixtureOptions struct {
	newAccount           bool
	storeObjects         []objectstore.TestObject
	createFirstSpaceHook func(ctx context.Context) error
}

type fixture struct {
	*service
	a           *app.App
	spaceCore   *mock_spacecore.MockSpaceCoreService
	objectStore *objectstore.StoreFixture
	config      *config.Config
	accountKeys *accountdata.AccountKeys
	techMock    *mock_techspace.MockTechSpace
	provider    *fakeTechProvider
}

func newFixture(t *testing.T, opts fixtureOptions) *fixture {
	fx := &fixture{
		service:     New().(*service),
		a:           new(app.App),
		spaceCore:   mock_spacecore.NewMockSpaceCoreService(t),
		objectStore: objectstore.NewStoreFixture(t),
		config:      config.New(config.WithNewAccount(opts.newAccount)),
		techMock:    mock_techspace.NewMockTechSpace(t),
		provider:    &fakeTechProvider{},
	}
	fx.config.PeferYamuxTransport = true
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)
	fx.accountKeys = keys
	wallet := mock_wallet.NewMockWallet(t)
	repoPath, err := os.MkdirTemp("", "spacev2-repo")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(repoPath) })
	wallet.EXPECT().Account().Return(keys)
	wallet.EXPECT().RepoPath().Return(repoPath)

	fx.spaceCore.EXPECT().DeriveID(mock.Anything, spacedomain.SpaceTypeRegular).Return(testPersonalSpaceId, nil).Times(1)
	fx.spaceCore.EXPECT().DeriveID(mock.Anything, spacedomain.SpaceTypeTech).Return(testTechSpaceId, nil).Times(1)

	// Run reaches StartSync in both bootstrap paths.
	techSpace := &clientspace.TechSpace{TechSpace: fx.techMock}
	fx.techMock.EXPECT().StartSync().Times(1)
	if opts.newAccount {
		fx.provider.createResult = techSpace
	} else {
		fx.provider.loadResults = []loadResult{{ts: techSpace}}
	}

	// Test seams (Init keeps pre-set values): fake tech provider and a fake
	// marketplace controller so Run does not need the object-cache stack.
	fx.service.techProvider = fx.provider
	fx.service.marketplace = &fakeController{spaceId: addr.AnytypeMarketplaceWorkspace}
	fx.service.createFirstSpaceHook = opts.createFirstSpaceHook

	if len(opts.storeObjects) > 0 {
		fx.objectStore.AddObjects(t, testTechSpaceId, opts.storeObjects)
	}

	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Maybe()
	notificationSender := mock_space.NewMockNotificationSender(t)

	fx.a.
		Register(testutil.PrepareMock(ctx, fx.a, wallet)).
		Register(fx.config).
		Register(fx.objectStore).
		Register(testutil.PrepareMock(ctx, fx.a, mock_kanban.NewMockService(t))).
		Register(testutil.PrepareMock(ctx, fx.a, eventSender)).
		Register(&dummyCollectionService{}).
		Register(subscription.New()).
		Register(testutil.PrepareMock(ctx, fx.a, notificationSender)).
		Register(&testSpaceLoaderListener{}).
		Register(testutil.PrepareMock(ctx, fx.a, fx.spaceCore)).
		Register(fx.service)

	require.NoError(t, fx.a.Start(ctx))
	t.Cleanup(func() {
		require.NoError(t, fx.a.Close(ctx))
	})
	return fx
}

func givenSpaceViewObject(viewId, targetSpaceId string, accountStatus spaceinfo.AccountStatus) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                 domain.String(viewId),
		bundle.RelationKeyTargetSpaceId:      domain.String(targetSpaceId),
		bundle.RelationKeyResolvedLayout:     domain.Int64(int64(model.ObjectType_spaceView)),
		bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(accountStatus)),
		bundle.RelationKeySpaceLocalStatus:   domain.Int64(int64(spaceinfo.LocalStatusOk)),
		bundle.RelationKeySpaceRemoteStatus:  domain.Int64(int64(spaceinfo.RemoteStatusOk)),
	}
}

func registryEntryState(r *registry, spaceId string) (entryState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.entries[spaceId]
	if !ok {
		return statePlaceholder, nil
	}
	return e.state, e.err
}

func TestService_Init(t *testing.T) {
	t.Run("derives ids and account metadata identical to the v1 derivations", func(t *testing.T) {
		// given
		fx := newFixture(t, fixtureOptions{})

		// then: ids come from the stubbed DeriveID calls (same calls v1 makes)
		assert.Equal(t, testPersonalSpaceId, fx.PersonalSpaceId())
		assert.Equal(t, testTechSpaceId, fx.TechSpaceId())

		// parity oracle: metadata payload/symkey equal the direct v1 derivation
		wantMeta, wantKey, err := domain.DeriveAccountMetadata(fx.accountKeys.SignKey)
		require.NoError(t, err)
		wantPayload, err := wantMeta.Marshal()
		require.NoError(t, err)
		assert.Equal(t, wantPayload, fx.AccountMetadataPayload())
		assert.Equal(t, wantKey, fx.AccountMetadataSymKey())

		// repKey parses the personal space id suffix (v1 getRepKey parity)
		want, err := getRepKey(testPersonalSpaceId)
		require.NoError(t, err)
		assert.Equal(t, want, fx.repKey)
	})
}

func TestService_Run(t *testing.T) {
	t.Run("existing account enumerates space views into the registry", func(t *testing.T) {
		// given: three space views persisted in the tech space store
		spaceIds := []string{"space1.1", "space2.2", "space3.3"}
		fx := newFixture(t, fixtureOptions{
			storeObjects: []objectstore.TestObject{
				givenSpaceViewObject("view1", spaceIds[0], spaceinfo.AccountStatusActive),
				givenSpaceViewObject("view2", spaceIds[1], spaceinfo.AccountStatusActive),
				givenSpaceViewObject("view3", spaceIds[2], spaceinfo.AccountStatusJoining),
			},
		})

		// then: the watcher replay reaches the single build path for each space
		// (M2 gate: subscription → dedup → apply → registry; the builder itself
		// is the M3 milestone, so entries fail with errBuilderNotImplemented)
		for _, spaceId := range spaceIds {
			require.Eventually(t, func() bool {
				state, err := registryEntryState(fx.registry, spaceId)
				return state == stateFailed && errors.Is(err, errBuilderNotImplemented)
			}, 2*time.Second, 10*time.Millisecond, "space %s not applied", spaceId)
		}

		// marketplace is a static ready entry
		state, err := registryEntryState(fx.registry, addr.AnytypeMarketplaceWorkspace)
		require.NoError(t, err)
		assert.Equal(t, stateReady, state)
	})

	t.Run("new account creates tech space and first space", func(t *testing.T) {
		// given
		var firstSpaceCreated bool
		fx := newFixture(t, fixtureOptions{
			newAccount: true,
			createFirstSpaceHook: func(ctx context.Context) error {
				firstSpaceCreated = true
				return nil
			},
		})

		// then
		assert.True(t, firstSpaceCreated)
		assert.Equal(t, 1, fx.provider.createCalls)
		assert.Equal(t, 0, fx.provider.loadCalls)
	})
}

func TestService_ApplyOnExistingController(t *testing.T) {
	t.Run("subsequent events drive Update, not a rebuild", func(t *testing.T) {
		// given: a ready controller in the registry
		s := &service{registry: newRegistry()}
		s.ctx, s.ctxCancel = context.WithCancel(context.Background())
		defer s.ctxCancel()
		fake := &fakeController{spaceId: "spaceA"}
		s.registry.addStatic("spaceA", fake)

		// when: a watcher event arrives for it
		s.onSpaceStatusUpdated(spaceViewStatus{spaceId: "spaceA", accountStatus: spaceinfo.AccountStatusActive})

		// then: the live controller is updated (§9.2: Update re-reads live state)
		require.Eventually(t, func() bool {
			return fake.updateCount() == 1
		}, time.Second, 5*time.Millisecond)
	})
}

func TestService_ViewUpdateReachesController(t *testing.T) {
	t.Run("a SpaceView detail change flows through the subscription to Update", func(t *testing.T) {
		// given: one space view replayed at startup
		spaceId := "space1.1"
		fx := newFixture(t, fixtureOptions{
			storeObjects: []objectstore.TestObject{
				givenSpaceViewObject("view1", spaceId, spaceinfo.AccountStatusActive),
			},
		})
		require.Eventually(t, func() bool {
			state, err := registryEntryState(fx.registry, spaceId)
			return state == stateFailed && errors.Is(err, errBuilderNotImplemented)
		}, 2*time.Second, 10*time.Millisecond)

		// and: its controller now exists (simulating the M3 builder result)
		fake := &fakeController{spaceId: spaceId}
		fx.registry.addStatic(spaceId, fake)

		// when: the view's account status changes in the store (UpdateKeys path)
		fx.objectStore.AddObjects(t, testTechSpaceId, []objectstore.TestObject{
			givenSpaceViewObject("view1", spaceId, spaceinfo.AccountStatusDeleted),
		})

		// then: the update reaches the existing controller
		require.Eventually(t, func() bool {
			return fake.updateCount() >= 1
		}, 2*time.Second, 10*time.Millisecond)
	})
}

func TestConvertSpaceError(t *testing.T) {
	// the documented error set is a caller-facing contract (§5.1)
	assert.ErrorIs(t, convertSpaceError(fmt.Errorf("wrap: %w", spacesyncproto.ErrSpaceIsDeleted)), ErrSpaceDeleted)
	assert.ErrorIs(t, convertSpaceError(fmt.Errorf("wrap: %w", spacestorage.ErrSpaceStorageMissing)), ErrSpaceStorageMissig)
	other := errors.New("other")
	assert.ErrorIs(t, convertSpaceError(other), other)
}

func TestService_Close(t *testing.T) {
	t.Run("close stops the apply path and releases waiters", func(t *testing.T) {
		// given
		s := &service{registry: newRegistry()}
		s.ctx, s.ctxCancel = context.WithCancel(context.Background())
		require.NoError(t, s.Close(ctx))

		// when: a late watcher event arrives after Close
		s.onSpaceStatusUpdated(spaceViewStatus{spaceId: "late.space"})

		// then: no controller entry is ever built for it
		time.Sleep(20 * time.Millisecond)
		_, err := s.registry.get(ctx, "late.space")
		require.ErrorIs(t, err, ErrSpaceNotExists)
		_, err = s.registry.await(ctx, "late.space")
		require.ErrorIs(t, err, ErrSpaceIsClosing)
	})
}
