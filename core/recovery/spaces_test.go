package recovery

import (
	"context"
	"errors"
	"testing"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacedomain"
)

// ready brings a fixture to LoadingSpaces with the tech space known and loaded.
func (fx *fixture) ready(t *testing.T) {
	t.Helper()
	fx.init(t)
	fx.OnTechSpaceId(techSpaceId)
	fx.OnAccountReady()
	fx.OnSpaceViewsInitial(nil)
	require.Equal(t, pb.EventAccountRecovery_LoadingSpaces, fx.Snapshot().Phase)
}

func (fx *fixture) space(t *testing.T, spaceId string) *pb.EventAccountRecoverySnapshotSpace {
	t.Helper()
	for _, s := range fx.Snapshot().Spaces {
		if s.SpaceId == spaceId {
			return s
		}
	}
	t.Fatalf("space %s not in snapshot", spaceId)
	return nil
}

func (fx *fixture) hasSpace(spaceId string) bool {
	for _, s := range fx.Snapshot().Spaces {
		if s.SpaceId == spaceId {
			return true
		}
	}
	return false
}

func (fx *fixture) stateChanges(spaceId string) []*pb.EventAccountRecoverySpaceStateChanged {
	var out []*pb.EventAccountRecoverySpaceStateChanged
	for _, u := range fx.sender.updates() {
		if p, ok := u.Payload.(*pb.EventAccountRecoveryUpdatePayloadOfSpaceStateChanged); ok && p.SpaceStateChanged.SpaceId == spaceId {
			out = append(out, p.SpaceStateChanged)
		}
	}
	return out
}

func (fx *fixture) finished(t *testing.T) *pb.EventAccountRecoveryFinished {
	t.Helper()
	for _, u := range fx.sender.updates() {
		if p, ok := u.Payload.(*pb.EventAccountRecoveryUpdatePayloadOfFinished); ok {
			return p.Finished
		}
	}
	return nil
}

func TestTracker_SpaceDiscovery(t *testing.T) {
	t.Run("a space view announces the space as Queued, once", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		before := len(fx.sender.updates())

		// when
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceView("s1", "v1", false)

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, before+1)
		want := &pb.EventAccountRecoverySpaceDiscovered{SpaceId: "s1", SpaceViewId: "v1", Kind: pb.EventAccountRecovery_Regular}
		assert.Equal(t, want, ups[before].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfSpaceDiscovered).SpaceDiscovered)
		assert.Equal(t, pb.EventAccountRecovery_Queued, fx.space(t, "s1").State)
		assert.Equal(t, int32(2), fx.Snapshot().SpacesTotal, "tech + s1")
	})

	t.Run("a deleted view drops a tracked space with Removed", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceLoadStarted("s1", false)

		// when
		fx.OnSpaceView("s1", "v1", true)

		// then
		changes := fx.stateChanges("s1")
		require.Len(t, changes, 2)
		want := &pb.EventAccountRecoverySpaceStateChanged{
			SpaceId: "s1", State: pb.EventAccountRecovery_Removed, FromState: pb.EventAccountRecovery_Loading,
			Error: &pb.EventAccountRecoveryErrorInfo{Class: pb.EventAccountRecovery_SpaceDeleted, DebugMessage: "space deleted while recovering"},
		}
		assert.Equal(t, want, changes[1])
		assert.False(t, fx.hasSpace("s1"))
		snap := fx.Snapshot()
		assert.Equal(t, int32(1), snap.SpacesTotal)
		assert.Equal(t, int32(0), snap.SpacesFailed)
	})

	t.Run("a deleted view for an unknown space, the tech space id and empty ids are silent", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		before := len(fx.sender.updates())

		// when
		fx.OnSpaceView("unknown", "v9", true)
		fx.OnSpaceView(techSpaceId, "vt", false)
		fx.OnSpaceView("", "v0", false)

		// then
		assert.Len(t, fx.sender.updates(), before)
		assert.Equal(t, int32(1), fx.Snapshot().SpacesTotal)
	})

	t.Run("a load seen before the view announces without an id; the view fills it in", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)

		// when
		fx.OnSpaceLoadStarted("s1", false)
		fx.OnSpaceView("s1", "v1", false)

		// then
		var discovered []*pb.EventAccountRecoverySpaceDiscovered
		for _, u := range fx.sender.updates() {
			if p, ok := u.Payload.(*pb.EventAccountRecoveryUpdatePayloadOfSpaceDiscovered); ok && p.SpaceDiscovered.SpaceId == "s1" {
				discovered = append(discovered, p.SpaceDiscovered)
			}
		}
		require.Len(t, discovered, 2)
		assert.Equal(t, "", discovered[0].SpaceViewId)
		assert.Equal(t, "v1", discovered[1].SpaceViewId)
		s := fx.space(t, "s1")
		assert.Equal(t, "v1", s.SpaceViewId)
		assert.Equal(t, pb.EventAccountRecovery_Loading, s.State)
	})
}

func TestTracker_SpaceLoad(t *testing.T) {
	t.Run("cold path: Loading then Loaded", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnSpaceView("s1", "v1", false)

		// when
		fx.OnSpaceLoadStarted("s1", false)
		fx.OnSpaceLoaded("s1", nil, false)

		// then
		changes := fx.stateChanges("s1")
		require.Len(t, changes, 2)
		assert.Equal(t, pb.EventAccountRecovery_Loading, changes[0].State)
		assert.Equal(t, pb.EventAccountRecovery_Queued, changes[0].FromState)
		assert.Equal(t, pb.EventAccountRecovery_Loaded, changes[1].State)
		assert.Equal(t, pb.EventAccountRecovery_Loading, changes[1].FromState)
		assert.Equal(t, int32(2), fx.Snapshot().SpacesLoaded)
	})

	t.Run("optimistic path: straight to Loaded, and a later result is idempotent", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)
		fx.OnSpaceView("s1", "v1", false)

		// when
		fx.OnSpaceLoadStarted("s1", true)
		fx.OnSpaceLoaded("s1", nil, false)

		// then
		changes := fx.stateChanges("s1")
		require.Len(t, changes, 1)
		assert.Equal(t, pb.EventAccountRecovery_Loaded, changes[0].State)
		assert.Equal(t, pb.EventAccountRecovery_Queued, changes[0].FromState)
	})

	t.Run("cancelled builds leave the state untouched", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceLoadStarted("s1", false)

		// when
		fx.OnSpaceLoaded("s1", context.Canceled, false)
		fx.OnSpaceLoaded("s1", context.DeadlineExceeded, false)

		// then
		assert.Len(t, fx.stateChanges("s1"), 1)
		assert.Equal(t, pb.EventAccountRecovery_Loading, fx.space(t, "s1").State)
	})

	t.Run("the loader's deletion verdict is a non-retryable SpaceDeleted error", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceLoadStarted("s1", false)

		// when
		fx.OnSpaceLoaded("s1", errors.New("space is deleted"), true)

		// then
		s := fx.space(t, "s1")
		assert.Equal(t, pb.EventAccountRecovery_Error, s.State)
		assert.Equal(t, pb.EventAccountRecovery_SpaceDeleted, s.Error.Class)
		assert.False(t, s.Error.Retryable)
		assert.Equal(t, int32(1), fx.Snapshot().SpacesFailed)
	})

	t.Run("other load errors classify", func(t *testing.T) {
		tests := []struct {
			name      string
			err       error
			class     pb.EventAccountRecoveryErrorClass
			retryable bool
		}{
			{"corrupt store", anystore.ErrCollectionNotFound, pb.EventAccountRecovery_Unexpected, false},
			{"unexpected space type", spacedomain.ErrUnexpectedSpaceType, pb.EventAccountRecovery_Unexpected, false},
			{"unknown", errors.New("boom"), pb.EventAccountRecovery_Unexpected, true},
			{"unreachable", net.ErrUnableToConnect, pb.EventAccountRecovery_PeerUnreachable, true},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				// given
				fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
				fx.ready(t)
				fx.OnSpaceView("s1", "v1", false)

				// when
				fx.OnSpaceLoaded("s1", tc.err, false)

				// then
				s := fx.space(t, "s1")
				assert.Equal(t, pb.EventAccountRecovery_Error, s.State)
				assert.Equal(t, tc.class, s.Error.Class)
				assert.Equal(t, tc.retryable, s.Error.Retryable)
			})
		}
	})
}

func TestTracker_SpacePull(t *testing.T) {
	t.Run("a pull interrupts Loading and a nil result resumes it", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceLoadStarted("s1", false)

		// when
		fx.pull(commonspace.PullEventWaiting, "s1", "", nil)
		fx.pull(commonspace.PullEventAttempt, "s1", "node1", nil)
		fx.pull(commonspace.PullEventResult, "s1", "node1", nil)
		fx.OnSpaceLoaded("s1", nil, false)

		// then
		var states []pb.EventAccountRecoverySpaceState
		for _, c := range fx.stateChanges("s1") {
			states = append(states, c.State)
		}
		want := []pb.EventAccountRecoverySpaceState{
			pb.EventAccountRecovery_Loading, pb.EventAccountRecovery_Pulling, pb.EventAccountRecovery_Loading, pb.EventAccountRecovery_Loaded,
		}
		assert.Equal(t, want, states)
	})

	t.Run("failed results count attempts; repeats of a class coalesce, a new class is an edge", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnSpaceView("s1", "v1", false)
		fx.pull(commonspace.PullEventWaiting, "s1", "", nil)
		fx.flush()
		before := len(fx.sender.updates())

		// when
		fx.pull(commonspace.PullEventResult, "s1", "node1", net.ErrUnableToConnect)
		fx.pull(commonspace.PullEventResult, "s1", "node2", net.ErrUnableToConnect)
		require.Len(t, fx.sender.updates(), before+1, "same class: coalesced")
		fx.pull(commonspace.PullEventResult, "s1", "node3", errors.New("persist failed"))

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, before+3, "the new class flushes the pending level first")
		assert.Equal(t, int32(2), ups[before+1].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfSpaceStateChanged).SpaceStateChanged.Attempt)
		last := ups[before+2].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfSpaceStateChanged).SpaceStateChanged
		assert.Equal(t, int32(3), last.Attempt)
		assert.Equal(t, pb.EventAccountRecovery_Unexpected, last.Error.Class)
		s := fx.space(t, "s1")
		assert.Equal(t, pb.EventAccountRecovery_Pulling, s.State)
		assert.Equal(t, int32(3), s.Attempt)
	})
}

func TestTracker_ViewGate(t *testing.T) {
	loadedSpace := func(fx *fixture, id, viewId string) {
		fx.OnSpaceView(id, viewId, false)
		fx.OnSpaceLoadStarted(id, true)
	}

	t.Run("without a diff nothing finishes", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)
		loadedSpace(fx, "s1", "v1")

		// then
		assert.Nil(t, fx.finished(t))
		assert.False(t, fx.Snapshot().Done)
	})

	t.Run("views arriving after the diff open the gate, confirmed", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "v2"}, true)

		// when
		loadedSpace(fx, "s1", "v1")
		require.Nil(t, fx.finished(t), "v2 still missing")
		loadedSpace(fx, "s2", "v2")

		// then
		fin := fx.finished(t)
		require.NotNil(t, fin)
		assert.True(t, fin.ViewsConfirmed)
		assert.Equal(t, int32(3), fin.SpacesTotal)
		assert.Equal(t, int32(3), fin.SpacesLoaded)
	})

	t.Run("views seen before the diff count as present", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		loadedSpace(fx, "s1", "v1")

		// when
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1"}, true)

		// then
		require.NotNil(t, fx.finished(t))
		assert.True(t, fx.finished(t).ViewsConfirmed)
	})

	t.Run("a later diff drops a leftover non-view id", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		loadedSpace(fx, "s1", "v1")
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "accountObject"}, true)
		require.Nil(t, fx.finished(t))

		// when
		fx.OnHeadSync(techSpaceId, "node1", []string{}, true)

		// then
		require.NotNil(t, fx.finished(t))
		assert.True(t, fx.finished(t).ViewsConfirmed)
	})

	t.Run("a deleted view still resolves", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1"}, true)

		// when
		fx.OnSpaceView("s1", "v1", true)

		// then
		fin := fx.finished(t)
		require.NotNil(t, fin)
		assert.Equal(t, int32(1), fin.SpacesTotal, "the deleted space is not counted")
	})

	t.Run("non-responsible diffs are ignored, except in local-only mode", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)
		loadedSpace(fx, "s1", "v1")

		// when
		fx.OnHeadSync(techSpaceId, "lan1", []string{}, false)
		require.Nil(t, fx.finished(t))
		fx.mu.Lock()
		fx.run.localOnly = true
		fx.mu.Unlock()
		fx.OnHeadSync(techSpaceId, "lan1", []string{}, false)

		// then
		require.NotNil(t, fx.finished(t))
	})

	t.Run("diffs for regular spaces are ignored", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)
		loadedSpace(fx, "s1", "v1")

		// when
		fx.OnHeadSync("s1", "node1", []string{}, true)

		// then
		assert.Nil(t, fx.finished(t))
	})

	t.Run("the gate waits for the watcher's first pass, then for every view it listed", func(t *testing.T) {
		// given: the tech space is ready and the network's diff has already
		// landed — before the local SpaceViews were delivered
		fx := newFixture(t, pb.EventAccountRecovery_NewAccount)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.OnAccountReady()
		fx.OnHeadSync(techSpaceId, "node1", nil, true)
		require.Nil(t, fx.finished(t), "no first pass yet: the tech space alone must not finish the run")

		// when
		fx.OnSpaceViewsInitial([]string{"v1"})
		require.Nil(t, fx.finished(t), "v1 is listed locally but not delivered yet")
		loadedSpace(fx, "s1", "v1")

		// then
		fin := fx.finished(t)
		require.NotNil(t, fin)
		assert.True(t, fin.ViewsConfirmed)
		assert.Equal(t, int32(2), fin.SpacesTotal)
	})

	t.Run("an initial view that turns out deleted still counts as delivered", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.OnAccountReady()
		fx.OnSpaceViewsInitial([]string{"v1"})
		fx.OnHeadSync(techSpaceId, "node1", nil, true)
		require.Nil(t, fx.finished(t))

		// when
		fx.OnSpaceView("s1", "v1", true)

		// then
		require.NotNil(t, fx.finished(t))
	})

	t.Run("the stall bound opens the gate after two diffs that resolve nothing, unconfirmed", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		loadedSpace(fx, "s1", "v1")

		// when
		fx.OnHeadSync(techSpaceId, "node1", []string{"stuck"}, true)
		fx.OnHeadSync(techSpaceId, "node1", []string{"stuck"}, true)
		require.Nil(t, fx.finished(t), "one stalled diff is not enough")
		fx.OnHeadSync(techSpaceId, "node1", []string{"stuck"}, true)

		// then
		fin := fx.finished(t)
		require.NotNil(t, fin)
		assert.False(t, fin.ViewsConfirmed)
		assert.False(t, fx.Snapshot().ViewsConfirmed)
	})

	t.Run("a diff that resolves something resets the stall counter", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		loadedSpace(fx, "s1", "v1")
		fx.OnHeadSync(techSpaceId, "node1", []string{"a", "b"}, true)
		fx.OnHeadSync(techSpaceId, "node1", []string{"a", "b"}, true) // stalled: 1

		// when
		fx.OnHeadSync(techSpaceId, "node1", []string{"a"}, true) // shrank: reset
		fx.OnHeadSync(techSpaceId, "node1", []string{"a"}, true) // stalled: 1
		require.Nil(t, fx.finished(t))
		fx.OnHeadSync(techSpaceId, "node1", []string{"a"}, true) // stalled: 2

		// then
		require.NotNil(t, fx.finished(t))
		assert.False(t, fx.finished(t).ViewsConfirmed)
	})

	t.Run("a view arriving between diffs resets the stall counter", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "stuck"}, true)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "stuck"}, true) // stalled: 1

		// when
		loadedSpace(fx, "s1", "v1")                                  // resolves v1: reset
		fx.OnHeadSync(techSpaceId, "node1", []string{"stuck"}, true) // something resolved since the last diff: 0
		fx.OnHeadSync(techSpaceId, "node1", []string{"stuck"}, true) // stalled: 1
		require.Nil(t, fx.finished(t))
		fx.OnHeadSync(techSpaceId, "node1", []string{"stuck"}, true) // stalled: 2

		// then
		require.NotNil(t, fx.finished(t))
	})
}

func TestTracker_Finished(t *testing.T) {
	t.Run("needs the account, the gate and every space settled; then the run goes silent", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.OnSpaceViewsInitial(nil)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceView("s2", "v2", false)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "v2"}, true)
		fx.OnSpaceLoadStarted("s1", false)
		fx.OnSpaceLoaded("s1", nil, false)
		require.Nil(t, fx.finished(t), "account not ready")
		fx.clock.Advance(3 * time.Second)
		fx.OnAccountReady()
		require.Nil(t, fx.finished(t), "s2 not settled")

		// when
		fx.OnSpaceLoaded("s2", errors.New("corrupt"), false)

		// then
		want := &pb.EventAccountRecoveryFinished{SpacesTotal: 3, SpacesLoaded: 2, SpacesFailed: 1, TotalDurationMs: 3000, ViewsConfirmed: true}
		assert.Equal(t, want, fx.finished(t))
		changes := fx.phaseChanges()
		last := changes[len(changes)-1]
		assert.Equal(t, pb.EventAccountRecovery_Done, last.Phase)
		assert.Equal(t, pb.EventAccountRecovery_LoadingSpaces, last.FromPhase)
		ups := fx.sender.updates()
		_, isFinished := ups[len(ups)-2].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfFinished)
		assert.True(t, isFinished, "Finished precedes the Done edge")
		snap := fx.Snapshot()
		assert.True(t, snap.Done)
		assert.True(t, snap.ViewsConfirmed)
		assert.Equal(t, pb.EventAccountRecovery_Done, snap.Phase)

		// and nothing moves after Done
		count := len(ups)
		fx.OnSpaceView("s3", "v3", false)
		fx.OnSpaceLoaded("s2", nil, false)
		fx.dialStarted("node2", 1)
		fx.dialFailed("node2", net.ErrUnableToConnect, time.Second)
		fx.OnLocalPeerDiscovered("lan9", nil)
		fx.Fail(errors.New("late"))
		fx.clock.Advance(waitingForNetworkAfter)
		fx.flush()
		assert.Len(t, fx.sender.updates(), count)
		assert.Equal(t, snap, fx.Snapshot())
	})

	t.Run("a warm start online finishes on the first empty diff", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceLoadStarted("s1", true)

		// when
		fx.OnHeadSync(techSpaceId, "node1", nil, true)

		// then
		fin := fx.finished(t)
		require.NotNil(t, fin)
		assert.True(t, fin.ViewsConfirmed)
		assert.Equal(t, int32(2), fin.SpacesLoaded)
	})

	t.Run("an empty first pass and an empty diff finish on the tech space alone", func(t *testing.T) {
		// given: the store genuinely holds no SpaceView
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)

		// when
		fx.OnHeadSync(techSpaceId, "node1", nil, true)

		// then
		fin := fx.finished(t)
		require.NotNil(t, fin)
		assert.Equal(t, int32(1), fin.SpacesTotal)
	})
}
