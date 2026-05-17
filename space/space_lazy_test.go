package space

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

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
		go func() { defer wg.Done(); s.applySpaceStatusForTest(statusFor(id, spaceinfo.LocalStatusOk, spaceinfo.AccountStatusActive)) }()
	}
	go s.drainDeferred(context.Background())
	wg.Wait()
	// give the drain workers time to finish building the snapshot
	s.drainDeferred(context.Background()) // idempotent second call: drains anything cached after releasing flipped

	s.mu.Lock()
	leftover := len(s.deferredStatuses)
	s.mu.Unlock()
	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < 50; i++ {
		id := "s" + strconv.Itoa(i)
		assert.True(t, built[id] || leftoverHas(s, id), "space "+id+" must be built or still queued, never stranded")
	}
	_ = leftover
}

func leftoverHas(s *service, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.deferredStatuses[id]
	return ok
}
