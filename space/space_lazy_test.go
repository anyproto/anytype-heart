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
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller/mock_spacecontroller"
	"github.com/anyproto/anytype-heart/space/spacefactory/mock_spacefactory"
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
	s.regChanged = make(chan struct{})
	s.regErr = map[string]error{}
	s.preloadCh = make(chan struct{})
	s.personalSpaceId = "personal.id"
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

// TestStartStatus_LazyRegistersDormant: in lazy mode a non-preferred space is
// registered (controller constructed, status-driven work possible) but not
// started — Start would begin loading, which must wait for demand.
func TestStartStatus_LazyRegistersDormant(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	factory := mock_spacefactory.NewMockSpaceFactory(t)
	// no Start expectation: the strict mock fails the test if Start is called
	ctrl := mock_spacecontroller.NewMockSpaceController(t)
	factory.EXPECT().NewShareableSpace(mock.Anything, "other", mock.Anything).Return(ctrl, nil)
	s.factory = factory

	got, err := s.startStatus(context.Background(), spaceinfo.NewSpacePersistentInfo("other"))
	require.NoError(t, err)
	require.NotNil(t, got)

	s.mu.Lock()
	_, registered := s.spaceControllers["other"]
	s.mu.Unlock()
	assert.True(t, registered, "non-preferred space must be registered dormant")
}

// TestStartStatus_LazyStartsPreferred: the preferred space starts (loads)
// immediately even in lazy mode.
func TestStartStatus_LazyStartsPreferred(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	factory := mock_spacefactory.NewMockSpaceFactory(t)
	ctrl := mock_spacecontroller.NewMockSpaceController(t)
	ctrl.EXPECT().Start(mock.Anything).Return(nil).Once()
	factory.EXPECT().NewShareableSpace(mock.Anything, "preferred", mock.Anything).Return(ctrl, nil)
	s.factory = factory

	_, err := s.startStatus(context.Background(), spaceinfo.NewSpacePersistentInfo("preferred"))
	require.NoError(t, err)
}

// TestStartStatus_EagerStartsEverything guards the backward-compat promise:
// with no preferred space (lazyMode=false) every space starts immediately.
func TestStartStatus_EagerStartsEverything(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = false

	factory := mock_spacefactory.NewMockSpaceFactory(t)
	s.factory = factory
	for i := 0; i < 3; i++ {
		id := "s" + strconv.Itoa(i)
		ctrl := mock_spacecontroller.NewMockSpaceController(t)
		ctrl.EXPECT().Start(mock.Anything).Return(nil).Once()
		factory.EXPECT().NewShareableSpace(mock.Anything, id, mock.Anything).Return(ctrl, nil)
		_, err := s.startStatus(context.Background(), spaceinfo.NewSpacePersistentInfo(id))
		require.NoError(t, err)
	}
}

// TestStartStatus_AfterReleaseStartsImmediately: once the backlog is released,
// newly registered spaces start immediately even in lazy mode.
func TestStartStatus_AfterReleaseStartsImmediately(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"
	s.releaseAll()

	factory := mock_spacefactory.NewMockSpaceFactory(t)
	ctrl := mock_spacecontroller.NewMockSpaceController(t)
	ctrl.EXPECT().Start(mock.Anything).Return(nil).Once()
	factory.EXPECT().NewShareableSpace(mock.Anything, "late", mock.Anything).Return(ctrl, nil)
	s.factory = factory

	_, err := s.startStatus(context.Background(), spaceinfo.NewSpacePersistentInfo("late"))
	require.NoError(t, err)
}

// TestReleaseAll_DemandsRegistered: releasing demands every dormant controller.
func TestReleaseAll_DemandsRegistered(t *testing.T) {
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	ctrlA := mock_spacecontroller.NewMockSpaceController(t)
	ctrlA.EXPECT().Demand().Once()
	ctrlB := mock_spacecontroller.NewMockSpaceController(t)
	ctrlB.EXPECT().Demand().Once()
	s.mu.Lock()
	s.spaceControllers["a"] = ctrlA
	s.spaceControllers["b"] = ctrlB
	s.mu.Unlock()

	s.releaseAll()

	s.mu.Lock()
	released := s.released
	s.mu.Unlock()
	assert.True(t, released)
}

// TestStartStatus_RegisterReleaseRace: a registration racing releaseAll must
// never strand a space dormant — it is demanded either by the release
// snapshot, by the demand decision (released already set), or by the
// late-release re-check after insertion.
func TestStartStatus_RegisterReleaseRace(t *testing.T) {
	const n = 50
	s := newLazyServiceForStatus(t)
	s.lazyMode = true
	s.preferredSpaceId = "preferred"

	var demanded sync.Map
	factory := mock_spacefactory.NewMockSpaceFactory(t)
	factory.EXPECT().NewShareableSpace(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(
		func(_ context.Context, id string, _ spaceinfo.SpacePersistentInfo) (spacecontroller.SpaceController, error) {
			ctrl := mock_spacecontroller.NewMockSpaceController(t)
			ctrl.EXPECT().Start(mock.Anything).RunAndReturn(func(context.Context) error {
				demanded.Store(id, true)
				return nil
			}).Maybe()
			ctrl.EXPECT().Demand().Run(func() {
				demanded.Store(id, true)
			}).Maybe()
			return ctrl, nil
		})
	s.factory = factory

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		id := "s" + strconv.Itoa(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.startStatus(context.Background(), spaceinfo.NewSpacePersistentInfo(id))
			require.NoError(t, err)
		}()
	}
	releaseDone := make(chan struct{})
	go func() {
		s.releaseAll()
		close(releaseDone)
	}()
	wg.Wait()
	<-releaseDone

	for i := 0; i < n; i++ {
		id := "s" + strconv.Itoa(i)
		_, ok := demanded.Load(id)
		assert.True(t, ok, "space "+id+" must be demanded, never stranded dormant")
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

// statusForRemote builds a spaceViewStatus with an explicit remoteStatus
// (statusFor hardcodes RemoteStatusOk).
func statusForRemote(spaceId string, local spaceinfo.LocalStatus, account spaceinfo.AccountStatus, remote spaceinfo.RemoteStatus) spaceViewStatus {
	st := statusFor(spaceId, local, account)
	st.remoteStatus = remote
	return st
}

// TestOnSpaceStatusUpdated_PreferredBroken_AllVariants covers every dynamic
// fallback trigger plus the non-trigger guards: a broken preferred space
// collapses lazy mode and releases the backlog.
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
