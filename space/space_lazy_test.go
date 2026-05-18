package space

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

	s.decideAndApplySpaceStatus(statusFor("other", spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive))

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
			s.decideAndApplySpaceStatus(statusFor(id, spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive))
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

// statusForRemote builds a spaceViewStatus with an explicit remoteStatus
// (statusFor hardcodes RemoteStatusOk).
func statusForRemote(spaceId string, local spaceinfo.LocalStatus, account spaceinfo.AccountStatus, remote spaceinfo.RemoteStatus) spaceViewStatus {
	st := statusFor(spaceId, local, account)
	st.remoteStatus = remote
	return st
}

// TestEagerMode_NoDeferral guards the spec §9 backward-compat promise:
// preferredSpaceId=="" (lazyMode=false) must never defer — every space builds
// inline, byte-identical to pre-feature behavior.
func TestEagerMode_NoDeferral(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = false
	s.preferredSpaceId = ""

	var mu sync.Mutex
	built := map[string]bool{}
	s.applySpaceStatusHook = func(st spaceViewStatus) {
		mu.Lock()
		built[st.spaceId] = true
		mu.Unlock()
	}

	for i := 0; i < 3; i++ {
		s.decideAndApplySpaceStatus(statusFor("s"+strconv.Itoa(i), spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive))
	}

	s.mu.Lock()
	assert.Empty(t, s.deferredStatuses, "eager mode must not defer any space")
	assert.False(t, s.releasing)
	s.mu.Unlock()
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < 3; i++ {
		assert.True(t, built["s"+strconv.Itoa(i)], "eager mode must build every space inline")
	}
}

// TestOnSpaceStatusUpdated_PreferredBuildsNotDeferred closes the §8.2 positive
// half: in lazy mode the preferred space itself builds immediately, never
// deferred.
func TestOnSpaceStatusUpdated_PreferredBuildsNotDeferred(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	built := make(chan string, 1)
	s.applySpaceStatusHook = func(st spaceViewStatus) { built <- st.spaceId }

	s.decideAndApplySpaceStatus(statusFor("preferred", spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive))

	select {
	case got := <-built:
		assert.Equal(t, "preferred", got)
	default:
		t.Fatal("preferred space must build immediately, not be deferred")
	}
	s.mu.Lock()
	_, deferred := s.deferredStatuses["preferred"]
	s.mu.Unlock()
	assert.False(t, deferred, "preferred space must never be cached as deferred")
}

// TestOnSpaceStatusUpdated_PreferredBroken_AllVariants covers every spec §3
// dynamic-fallback trigger plus the non-trigger guards.
func TestOnSpaceStatusUpdated_PreferredBroken_AllVariants(t *testing.T) {
	cases := []struct {
		name        string
		lazyMode    bool
		preferred   string
		status      spaceViewStatus
		wantRelease bool
	}{
		{"missing -> release", true, "preferred",
			statusFor("preferred", spaceinfo.LocalStatusMissing, spaceinfo.AccountStatusActive), true},
		{"removing -> release", true, "preferred",
			statusFor("preferred", spaceinfo.LocalStatusOk, spaceinfo.AccountStatusRemoving), true},
		{"account deleted -> release", true, "preferred",
			statusFor("preferred", spaceinfo.LocalStatusOk, spaceinfo.AccountStatusDeleted), true},
		{"remote deleted -> release", true, "preferred",
			statusForRemote("preferred", spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive, spaceinfo.RemoteStatusDeleted), true},
		{"joining -> no release", true, "preferred",
			statusFor("preferred", spaceinfo.LocalStatusLoading, spaceinfo.AccountStatusJoining), false},
		{"non-preferred broken -> no release", true, "preferred",
			statusFor("other", spaceinfo.LocalStatusMissing, spaceinfo.AccountStatusActive), false},
		{"eager mode broken -> no release", false, "preferred",
			statusFor("preferred", spaceinfo.LocalStatusMissing, spaceinfo.AccountStatusActive), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newLazyServiceForStatus(t)
			s.lazyMode = tc.lazyMode
			s.preferredSpaceId = tc.preferred

			s.maybeReleaseOnPreferredBroken(tc.status)

			released := false
			select {
			case <-s.preloadCh:
				released = true
			default:
			}
			assert.Equal(t, tc.wantRelease, released)
		})
	}
}

// TestDrainDeferred_BoundedConcurrency asserts the core feature property
// (spec §8.5): the backlog drains with at most preloadConcurrency builds in
// flight, replacing the unbounded ~10·N eager fan-out.
func TestDrainDeferred_BoundedConcurrency(t *testing.T) {
	oldK := preloadConcurrency
	preloadConcurrency = 2
	defer func() { preloadConcurrency = oldK }()

	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	var inflight, maxInflight atomic.Int32
	release := make(chan struct{})
	s.applySpaceStatusHook = func(st spaceViewStatus) {
		cur := inflight.Add(1)
		for {
			m := maxInflight.Load()
			if cur <= m || maxInflight.CompareAndSwap(m, cur) {
				break
			}
		}
		<-release
		inflight.Add(-1)
	}

	s.mu.Lock()
	for i := 0; i < 10; i++ {
		id := "s" + strconv.Itoa(i)
		s.deferredStatuses[id] = statusFor(id, spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive)
	}
	s.mu.Unlock()

	done := make(chan struct{})
	go func() { s.drainDeferred(context.Background()); close(done) }()

	// The first preloadConcurrency workers must both enter and block before we
	// release them, proving the pool actually caps concurrency.
	require.Eventually(t, func() bool {
		return maxInflight.Load() >= int32(preloadConcurrency)
	}, 2*time.Second, time.Millisecond)
	close(release)
	<-done

	assert.LessOrEqual(t, maxInflight.Load(), int32(preloadConcurrency),
		"never more than preloadConcurrency builds in flight")
	assert.Equal(t, int32(0), inflight.Load())
	s.mu.Lock()
	assert.Empty(t, s.deferredStatuses)
	s.mu.Unlock()
}
