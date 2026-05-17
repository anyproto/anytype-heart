package space

import (
	"context"
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
