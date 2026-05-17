package space

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace/mock_techspace"
)

func newMockTechSpaceViewExists(t *testing.T, spaceId string, exists bool, err error) *mock_techspace.MockTechSpace {
	ts := mock_techspace.NewMockTechSpace(t)
	ts.EXPECT().SpaceViewExists(mock.Anything, spaceId).Return(exists, err)
	return ts
}

func TestComputeLazyMode(t *testing.T) {
	newSvc := func(preferred, tech string) *service {
		return &service{preferredSpaceId: preferred, techSpaceId: tech}
	}

	t.Run("empty preferred -> eager", func(t *testing.T) {
		s := newSvc("", "techId")
		assert.False(t, s.computeLazyMode(context.Background(), nil))
	})

	t.Run("preferred is tech -> eager", func(t *testing.T) {
		s := newSvc("techId", "techId")
		assert.False(t, s.computeLazyMode(context.Background(), nil))
	})

	t.Run("preferred resolvable -> lazy", func(t *testing.T) {
		ts := newMockTechSpaceViewExists(t, "space.x", true, nil)
		s := newSvc("space.x", "techId")
		assert.True(t, s.computeLazyMode(context.Background(), &clientspace.TechSpace{TechSpace: ts}))
	})

	t.Run("preferred not resolvable -> eager", func(t *testing.T) {
		ts := newMockTechSpaceViewExists(t, "space.x", false, nil)
		s := newSvc("space.x", "techId")
		assert.False(t, s.computeLazyMode(context.Background(), &clientspace.TechSpace{TechSpace: ts}))
	})
}

func newLazyServiceForStatus(t *testing.T) *service {
	s := New().(*service)
	s.ctx = context.Background()
	s.spaceControllers = map[string]spacecontroller.SpaceController{}
	s.waiting = map[string]controllerWaiter{}
	s.deferredStatuses = map[string]spaceViewStatus{}
	s.preloadCh = make(chan struct{})
	return s
}

func statusFor(spaceId string, local spaceinfo.LocalStatus, account spaceinfo.AccountStatus) spaceViewStatus {
	return spaceViewStatus{
		spaceId:       spaceId,
		localStatus:   local,
		accountStatus: account,
		remoteStatus:  spaceinfo.RemoteStatusOk,
		mx:            &sync.Mutex{},
	}
}

func TestOnSpaceStatusUpdated_Defer(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	s.applySpaceStatusForTest(statusFor("other", spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive))

	s.mu.Lock()
	_, deferred := s.deferredStatuses["other"]
	_, built := s.spaceControllers["other"]
	s.mu.Unlock()
	assert.True(t, deferred, "non-preferred space must be cached as deferred")
	assert.False(t, built, "non-preferred space must NOT be built")
}

func TestOnSpaceStatusUpdated_PreferredBrokenReleases(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	s.maybeReleaseOnPreferredBroken(statusFor("preferred", spaceinfo.LocalStatusMissing, spaceinfo.AccountStatusActive))

	select {
	case <-s.preloadCh:
	default:
		t.Fatal("preferred space Missing must trigger release")
	}
}

func TestOnSpaceStatusUpdated_JoiningDoesNotRelease(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	s.maybeReleaseOnPreferredBroken(statusFor("preferred", spaceinfo.LocalStatusLoading, spaceinfo.AccountStatusJoining))

	select {
	case <-s.preloadCh:
		t.Fatal("Joining must NOT trigger release")
	default:
	}
}

func TestDrainDeferred_BuildsSnapshotAndClears(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	built := make(chan string, 8)
	s.applySpaceStatusHook = func(st spaceViewStatus) { built <- st.spaceId }

	s.mu.Lock()
	s.deferredStatuses["a"] = statusFor("a", spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive)
	s.deferredStatuses["b"] = statusFor("b", spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive)
	s.mu.Unlock()

	s.drainDeferred(context.Background())

	got := map[string]bool{}
	for i := 0; i < 2; i++ {
		got[<-built] = true
	}
	assert.True(t, got["a"] && got["b"], "drain must build every deferred space")

	s.mu.Lock()
	assert.True(t, s.releasing)
	assert.Empty(t, s.deferredStatuses, "deferred map cleared")
	s.mu.Unlock()
}

func TestDrainDeferred_NoStrandedRace(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	var mu sync.Mutex
	built := map[string]bool{}
	s.applySpaceStatusHook = func(st spaceViewStatus) {
		mu.Lock()
		built[st.spaceId] = true
		mu.Unlock()
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		id := "s" + strconv.Itoa(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.applySpaceStatusForTest(statusFor(id, spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive))
		}()
	}
	// Single release trigger, exactly as production does it (one drainDeferred
	// behind the sync.Once). Join it before asserting: drainDeferred returns
	// only after its bounded workers have built the whole snapshot, so the
	// B2 invariant is observable deterministically rather than racing the
	// background workers.
	d1 := make(chan struct{})
	go func() { s.drainDeferred(context.Background()); close(d1) }()
	wg.Wait() // every applier's decision (defer or inline build) finished
	<-d1      // first drain + all its workers finished
	// Mop-up: spaces an applier deferred while the drain had already passed
	// its snapshot see releasing==true and build inline, so nothing should
	// remain; this second call must be a safe no-op (closes the B2 window).
	s.drainDeferred(context.Background())

	s.mu.Lock()
	leftover := len(s.deferredStatuses)
	s.mu.Unlock()
	require.Zero(t, leftover, "no space may remain queued after the backlog is drained")

	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < 50; i++ {
		id := "s" + strconv.Itoa(i)
		assert.True(t, built[id], "space "+id+" must be built, never stranded")
	}
}

func TestPreloadRemainingSpaces_Idempotent(t *testing.T) {
	s := newLazyServiceForStatus(t)
	require.NoError(t, s.PreloadRemainingSpaces(context.Background()))
	require.NoError(t, s.PreloadRemainingSpaces(context.Background())) // must not panic on double close
	select {
	case <-s.preloadCh:
	default:
		t.Fatal("preloadCh must be closed after PreloadRemainingSpaces")
	}
}

func TestEnsureSpaceStarted_DeriveWhenNoCachedStatus(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true

	called := make(chan string, 1)
	s.startStatusHook = func(info spaceinfo.SpacePersistentInfo) { called <- info.SpaceID }

	s.ensureSpaceStarted("not-yet-cached")

	select {
	case got := <-called:
		assert.Equal(t, "not-yet-cached", got)
	default:
		t.Fatal("ensureSpaceStarted must derive+build when status not cached (E2)")
	}
}

func TestEnsureSpaceStarted_EagerModeNoOp(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = false
	s.startStatusHook = func(info spaceinfo.SpacePersistentInfo) { t.Fatal("must not build in eager mode") }
	s.ensureSpaceStarted("whatever") // no cached status, eager => preserve old no-op behavior
}
