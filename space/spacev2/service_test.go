package spacev2

import (
	"context"
	"os"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban/mock_kanban"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/space/spacecore/mock_spacecore"
	"github.com/anyproto/anytype-heart/space/spacedomain"
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

type fixture struct {
	*service
	a           *app.App
	spaceCore   *mock_spacecore.MockSpaceCoreService
	objectStore *objectstore.StoreFixture
	config      *config.Config
	accountKeys *accountdata.AccountKeys
}

func newFixture(t *testing.T, newAccount bool) *fixture {
	fx := &fixture{
		service:     New().(*service),
		a:           new(app.App),
		spaceCore:   mock_spacecore.NewMockSpaceCoreService(t),
		objectStore: objectstore.NewStoreFixture(t),
		config:      config.New(config.WithNewAccount(newAccount)),
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

func TestService_Init(t *testing.T) {
	t.Run("derives ids and account metadata identical to the v1 derivations", func(t *testing.T) {
		// given
		fx := newFixture(t, false)

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
