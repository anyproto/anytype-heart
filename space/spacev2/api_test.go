package spacev2

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

// apiTechSpace fakes the techspace surface the API layer touches.
type apiTechSpace struct {
	techspace.TechSpace
	mu sync.Mutex

	viewExists       bool
	views            map[string]spaceinfo.SpacePersistentInfo // SpaceViewCreate calls
	persistentWrites []spaceinfo.SpacePersistentInfo          // SetPersistentInfo calls
}

func (t *apiTechSpace) SpaceViewExists(ctx context.Context, spaceId string) (bool, error) {
	return t.viewExists, nil
}

func (t *apiTechSpace) SpaceViewCreate(ctx context.Context, spaceId string, force bool, info spaceinfo.SpacePersistentInfo, desc *spaceinfo.SpaceDescription) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.views[spaceId]; ok && !force {
		return techspace.ErrSpaceViewExists
	}
	t.views[spaceId] = info
	return nil
}

func (t *apiTechSpace) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.persistentWrites = append(t.persistentWrites, info)
	return nil
}

type fakeNotificationSender struct {
	mu   sync.Mutex
	sent []*model.Notification
}

func (f *fakeNotificationSender) Init(a *app.App) error { return nil }
func (f *fakeNotificationSender) Name() string          { return "fakeNotifications" }
func (f *fakeNotificationSender) CreateAndSend(notification *model.Notification) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, notification)
	return nil
}

func (f *fakeNotificationSender) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

type fakeSpaceNameGetter struct{}

func (f *fakeSpaceNameGetter) GetSpaceName(spaceId string) string { return "space name" }

type fakeAccountService struct {
	accountservice.Service
	keys *accountdata.AccountKeys
}

func (f *fakeAccountService) Account() *accountdata.AccountKeys { return f.keys }

type apiFixture struct {
	svc  *service
	tech *apiTechSpace

	mu       sync.Mutex
	backends map[string]*fakeBackend

	notifications *fakeNotificationSender
}

func newAPIFixture(t *testing.T) *apiFixture {
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)

	fx := &apiFixture{
		tech:          &apiTechSpace{views: map[string]spaceinfo.SpacePersistentInfo{}},
		backends:      map[string]*fakeBackend{},
		notifications: &fakeNotificationSender{},
	}
	ready := make(chan struct{})
	close(ready)
	fx.svc = &service{
		techSpaceId:         "techSpaceId",
		personalSpaceId:     "personalSpaceId",
		techSpaceReady:      ready,
		techSpace:           &clientspace.TechSpace{TechSpace: fx.tech},
		marketplace:         &marketplaceSpace{vs: &fakeSpace{}, indexer: &marketplaceIndexer{}},
		preloadCh:           make(chan struct{}),
		notificationService: fx.notifications,
		spaceNameGetter:     &fakeSpaceNameGetter{},
		accountService:      &fakeAccountService{keys: keys},
	}
	fx.svc.ctx, fx.svc.ctxCancel = context.WithCancel(context.Background())
	fx.svc.registry = newRegistry(func(spaceId string) (*controller, error) {
		backend := newFakeBackend(spaceinfo.AccountStatusActive)
		fx.mu.Lock()
		fx.backends[spaceId] = backend
		fx.mu.Unlock()
		return newController(spaceId, backend, controllerOptions{
			retryMin: 2 * time.Millisecond,
			retryMax: 10 * time.Millisecond,
		}), nil
	})
	t.Cleanup(func() {
		fx.svc.ctxCancel()
		closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, fx.svc.registry.close(closeCtx))
	})
	return fx
}

func TestAPIGetUnknownSpace(t *testing.T) {
	fx := newAPIFixture(t)

	_, err := fx.svc.Get(context.Background(), "unknownSpace")

	assert.ErrorIs(t, err, ErrSpaceNotExists)
}

func TestAPIGetLoadsKnownSpace(t *testing.T) {
	// given: a discovered space (controller exists, not demanded)
	fx := newAPIFixture(t)
	_, err := fx.svc.registry.getOrCreate("space1")
	require.NoError(t, err)

	// when
	sp, err := fx.svc.Get(context.Background(), "space1")

	// then: Get promoted and loaded it
	require.NoError(t, err)
	require.NotNil(t, sp)
	assert.Equal(t, StateLoaded, fx.svc.registry.get("space1").State())
}

func TestAPIGetTechAndMarketplace(t *testing.T) {
	fx := newAPIFixture(t)

	ts, err := fx.svc.Get(context.Background(), "techSpaceId")
	require.NoError(t, err)
	assert.Same(t, fx.svc.techSpace, ts)

	mp, err := fx.svc.Get(context.Background(), addr.AnytypeMarketplaceWorkspace)
	require.NoError(t, err)
	assert.Same(t, fx.svc.marketplace.vs, mp)
}

func TestAPIWaitCreatesControllerWhenViewExists(t *testing.T) {
	// given: the view exists but no controller yet (e.g. not discovered)
	fx := newAPIFixture(t)
	fx.tech.viewExists = true

	// when
	sp, err := fx.svc.Wait(context.Background(), "space1")

	// then
	require.NoError(t, err)
	require.NotNil(t, sp)
	assert.NotNil(t, fx.svc.registry.get("space1"))
}

func TestAPIWaitUnknownSpace(t *testing.T) {
	fx := newAPIFixture(t)
	fx.tech.viewExists = false

	_, err := fx.svc.Wait(context.Background(), "space1")

	assert.ErrorIs(t, err, ErrSpaceNotExists)
}

func TestAPIJoinCreatesJoiningView(t *testing.T) {
	// given
	fx := newAPIFixture(t)

	// when
	require.NoError(t, fx.svc.Join(context.Background(), "space1", "aclHead1"))

	// then: view created with Joining + head, controller demanded
	info, ok := fx.tech.views["space1"]
	require.True(t, ok)
	assert.Equal(t, spaceinfo.AccountStatusJoining, info.GetAccountStatus())
	assert.Equal(t, "aclHead1", info.GetAclHeadId())
	ctrl := fx.svc.registry.get("space1")
	require.NotNil(t, ctrl)
	assert.True(t, ctrl.Wanted())
}

func TestAPIJoinExistingViewUpdatesStatus(t *testing.T) {
	// given: the view already exists
	fx := newAPIFixture(t)
	fx.tech.views["space1"] = spaceinfo.NewSpacePersistentInfo("space1")

	// when
	require.NoError(t, fx.svc.Join(context.Background(), "space1", "aclHead1"))

	// then: the status write went through SetPersistentInfo
	require.Len(t, fx.tech.persistentWrites, 1)
	assert.Equal(t, spaceinfo.AccountStatusJoining, fx.tech.persistentWrites[0].GetAccountStatus())
}

func TestAPIInviteJoinCreatesActiveView(t *testing.T) {
	fx := newAPIFixture(t)

	require.NoError(t, fx.svc.InviteJoin(context.Background(), "space1", "aclHead1"))

	info, ok := fx.tech.views["space1"]
	require.True(t, ok)
	assert.Equal(t, spaceinfo.AccountStatusActive, info.GetAccountStatus())
	assert.Equal(t, "aclHead1", info.GetAclHeadId())
}

func TestAPIDeleteWritesDeletedStatus(t *testing.T) {
	fx := newAPIFixture(t)

	require.NoError(t, fx.svc.Delete(context.Background(), "space1"))

	require.Len(t, fx.tech.persistentWrites, 1)
	assert.Equal(t, spaceinfo.AccountStatusDeleted, fx.tech.persistentWrites[0].GetAccountStatus())
}

func TestAPICancelLeaveWritesActiveStatus(t *testing.T) {
	fx := newAPIFixture(t)

	require.NoError(t, fx.svc.CancelLeave(context.Background(), "space1"))

	require.Len(t, fx.tech.persistentWrites, 1)
	assert.Equal(t, spaceinfo.AccountStatusActive, fx.tech.persistentWrites[0].GetAccountStatus())
}

func TestAPIEventRemoteDeletedReacts(t *testing.T) {
	// given: coordinator reports deleted, account status lags, space in use
	fx := newAPIFixture(t)
	ev := spaceViewEvent{
		spaceId:       "space1",
		accountStatus: spaceinfo.AccountStatusActive,
		localStatus:   spaceinfo.LocalStatusOk,
		remoteStatus:  spaceinfo.RemoteStatusDeleted,
	}

	// when
	fx.svc.onSpaceViewEvent(ev)

	// then: notification + AccountStatusDeleted write; no controller built
	require.Eventually(t, func() bool {
		fx.tech.mu.Lock()
		defer fx.tech.mu.Unlock()
		return len(fx.tech.persistentWrites) == 1
	}, time.Second, time.Millisecond)
	assert.Equal(t, spaceinfo.AccountStatusDeleted, fx.tech.persistentWrites[0].GetAccountStatus())
	assert.Equal(t, 1, fx.notifications.count())
	assert.Nil(t, fx.svc.registry.get("space1"))
}

func TestAPIEventEagerDiscoveryLoads(t *testing.T) {
	// given
	fx := newAPIFixture(t)
	ev := spaceViewEvent{spaceId: "space1", accountStatus: spaceinfo.AccountStatusActive}

	// when
	fx.svc.onSpaceViewEvent(ev)

	// then: controller created, demanded, loads
	ctrl := fx.svc.registry.get("space1")
	require.NotNil(t, ctrl)
	assert.True(t, ctrl.Wanted())
	waitState(t, ctrl, StateLoaded)
}

func TestAPIEventLazyDefersNonPreferred(t *testing.T) {
	// given: lazy mode with a preferred space
	fx := newAPIFixture(t)
	fx.svc.lazyMode = true
	fx.svc.preferredSpaceId = "preferredSpace"

	// when
	fx.svc.onSpaceViewEvent(spaceViewEvent{spaceId: "space1", accountStatus: spaceinfo.AccountStatusActive})
	fx.svc.onSpaceViewEvent(spaceViewEvent{spaceId: "preferredSpace", accountStatus: spaceinfo.AccountStatusActive})

	// then: both controllers exist, only the preferred one is demanded
	require.NotNil(t, fx.svc.registry.get("space1"))
	assert.False(t, fx.svc.registry.get("space1").Wanted())
	require.NotNil(t, fx.svc.registry.get("preferredSpace"))
	assert.True(t, fx.svc.registry.get("preferredSpace").Wanted())
}

func TestAPIPreloadDrainsDeferred(t *testing.T) {
	// given: deferred spaces in lazy mode
	fx := newAPIFixture(t)
	fx.svc.lazyMode = true
	fx.svc.preferredSpaceId = "preferredSpace"
	fx.svc.onSpaceViewEvent(spaceViewEvent{spaceId: "space1", accountStatus: spaceinfo.AccountStatusActive})
	fx.svc.onSpaceViewEvent(spaceViewEvent{spaceId: "space2", accountStatus: spaceinfo.AccountStatusActive})
	go fx.svc.drainDeferredLater()

	// when
	require.NoError(t, fx.svc.PreloadRemainingSpaces(context.Background()))

	// then: the backlog is promoted
	require.Eventually(t, func() bool {
		return fx.svc.registry.get("space1").Wanted() && fx.svc.registry.get("space2").Wanted()
	}, time.Second, time.Millisecond)
}

func TestAPIPreferredBrokenReleasesBacklog(t *testing.T) {
	// given
	fx := newAPIFixture(t)
	fx.svc.lazyMode = true
	fx.svc.preferredSpaceId = "preferredSpace"
	fx.svc.onSpaceViewEvent(spaceViewEvent{spaceId: "space1", accountStatus: spaceinfo.AccountStatusActive})
	go fx.svc.drainDeferredLater()

	// when: the preferred space turns out broken
	fx.svc.onSpaceViewEvent(spaceViewEvent{
		spaceId:       "preferredSpace",
		accountStatus: spaceinfo.AccountStatusActive,
		localStatus:   spaceinfo.LocalStatusMissing,
	})

	// then
	require.Eventually(t, func() bool {
		return fx.svc.registry.get("space1").Wanted()
	}, time.Second, time.Millisecond)
}

func TestAPIAllLoadedSpaceIds(t *testing.T) {
	// given: one loaded space, one idle
	fx := newAPIFixture(t)
	_, err := fx.svc.registry.getOrCreate("idleSpace")
	require.NoError(t, err)
	_, err = fx.svc.Get(context.Background(), func() string {
		_, _ = fx.svc.registry.getOrCreate("loadedSpace")
		return "loadedSpace"
	}())
	require.NoError(t, err)

	// then
	assert.Equal(t, []string{"loadedSpace"}, fx.svc.AllLoadedSpaceIds())
	assert.ElementsMatch(t, []string{"idleSpace", "loadedSpace"}, fx.svc.AllSpaceIds())
}

func TestAPIAddStreamableCreatesGuestView(t *testing.T) {
	// given
	fx := newAPIFixture(t)
	keys, err := accountdata.NewRandom()
	require.NoError(t, err)

	// when
	require.NoError(t, fx.svc.AddStreamable(context.Background(), "streamSpace", keys.SignKey))

	// then: the view carries the encoded guest key (what routes the space to
	// the streamable build path) and the controller is demanded
	info, ok := fx.tech.views["streamSpace"]
	require.True(t, ok)
	assert.Equal(t, spaceinfo.AccountStatusUnknown, info.GetAccountStatus())
	assert.NotEmpty(t, info.EncodedKey)
	ctrl := fx.svc.registry.get("streamSpace")
	require.NotNil(t, ctrl)
	assert.True(t, ctrl.Wanted())

	// and: idempotent for an existing view
	require.NoError(t, fx.svc.AddStreamable(context.Background(), "streamSpace", keys.SignKey))
}

func TestGetRepKey(t *testing.T) {
	key, err := getRepKey("bafyreib.2s8ujc9zx01ad")
	require.NoError(t, err)
	assert.NotZero(t, key)

	_, err = getRepKey("noseparator")
	assert.Error(t, err)
}
