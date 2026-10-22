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
	// loadedSpace is the on-disk fast path as it really runs: the loader shows
	// Loaded immediately, and the background build it still runs reports after.
	loadedSpace := func(fx *fixture, id, viewId string) {
		fx.OnSpaceView(id, viewId, false)
		fx.OnSpaceLoadStarted(id, true)
		fx.OnSpaceLoaded(id, nil, false)
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
		fx.OnSpaceLoaded("s1", nil, false)

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

// TestTracker_PendingJoinDoesNotWedgeFinished pins the terminal-class bug: a
// space whose controller runs a joiner rather than a spaceloader can never
// publish a load result, so tracking it as one awaiting a result holds
// Finished open for the entire run, on every app open.
func TestTracker_PendingJoinDoesNotWedgeFinished(t *testing.T) {
	// loadedSpace is the on-disk fast path as it really runs: the loader shows
	// Loaded immediately, and the background build it still runs reports after.
	loadedSpace := func(fx *fixture, id, viewId string) {
		fx.OnSpaceView(id, viewId, false)
		fx.OnSpaceLoadStarted(id, true)
		fx.OnSpaceLoaded(id, nil, false)
	}

	t.Run("the old routing wedges the run", func(t *testing.T) {
		// given: the diff names two views, and v2's space is a pending join
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "v2"}, true)
		loadedSpace(fx, "s1", "v1")

		// when: it arrives on the ordinary seam, as it used to
		fx.OnSpaceView("s2", "v2", false)

		// then: nothing can ever finish the run — no load result is coming
		assert.Nil(t, fx.finished(t))
		assert.False(t, fx.Snapshot().Done)
		assert.Equal(t, pb.EventAccountRecovery_Queued, fx.space(t, "s2").State)
	})

	t.Run("the inactive seam resolves the gate without being awaited", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "v2"}, true)
		loadedSpace(fx, "s1", "v1")
		require.Nil(t, fx.finished(t), "v2 still missing")

		// when
		fx.OnSpaceViewInactive("s2", "v2")

		// then
		fin := fx.finished(t)
		require.NotNil(t, fin, "a space that can never load must not hold the run open")
		assert.True(t, fin.ViewsConfirmed, "the view was accounted for, so completeness is still earned")
		assert.Equal(t, int32(2), fin.SpacesTotal, "tech + s1; a pending join is not being recovered")
		assert.False(t, fx.hasSpace("s2"))
	})

	t.Run("a tracked space that becomes a pending join leaves the set", func(t *testing.T) {
		// given: s2 was active and loading
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "v2"}, true)
		loadedSpace(fx, "s1", "v1")
		fx.OnSpaceView("s2", "v2", false)
		fx.OnSpaceLoadStarted("s2", false)
		require.Nil(t, fx.finished(t))

		// when
		fx.OnSpaceViewInactive("s2", "v2")

		// then: it is retired as Removed, and the run can finish
		changes := fx.stateChanges("s2")
		require.NotEmpty(t, changes)
		last := changes[len(changes)-1]
		assert.Equal(t, pb.EventAccountRecovery_Removed, last.State)
		assert.Equal(t, pb.EventAccountRecovery_None, last.Error.GetClass(), "nothing failed and nothing was deleted")
		require.NotNil(t, fx.finished(t))
	})
}

// TestTracker_SettleBound pins the terminal rule for a run that goes quiet
// without being able to claim completeness. The two outcomes are deliberately
// different: an account whose spaces all settled is ready (we just cannot
// promise there is nothing else to fetch), while one with a space that never
// reported is NOT ready and must not say so.
func TestTracker_SettleBound(t *testing.T) {
	// loadedSpace is the on-disk fast path as it really runs: the loader shows
	// Loaded immediately, and the background build it still runs reports after.
	loadedSpace := func(fx *fixture, id, viewId string) {
		fx.OnSpaceView(id, viewId, false)
		fx.OnSpaceLoadStarted(id, true)
		fx.OnSpaceLoaded(id, nil, false)
	}

	t.Run("all spaces settled with no diff finishes, unconfirmed", func(t *testing.T) {
		// given: a warm start with no connectivity — everything loads from disk
		// but no responsible diff ever arrives
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)
		loadedSpace(fx, "s1", "v1")
		require.Nil(t, fx.finished(t), "the gate has seen no diff")

		// when
		fx.clock.Advance(settleBound)

		// then
		fin := fx.finished(t)
		require.NotNil(t, fin, "a working offline app must not show progress forever")
		assert.False(t, fin.ViewsConfirmed, "no diff arrived, so completeness cannot be claimed")
		assert.Equal(t, int32(2), fin.SpacesLoaded)
		assert.True(t, fx.Snapshot().Done)
	})

	t.Run("a space that never reports goes Stalled and the run stays open", func(t *testing.T) {
		// given: the diff is in and every other space is loaded, but s2 never
		// publishes a load result
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "v2"}, true)
		loadedSpace(fx, "s1", "v1")
		fx.OnSpaceView("s2", "v2", false)
		fx.OnSpaceLoadStarted("s2", false)

		// when
		fx.clock.Advance(settleBound)

		// then
		assert.Equal(t, pb.EventAccountRecovery_Stalled, fx.space(t, "s2").State)
		assert.Nil(t, fx.finished(t), "the account is not recovered, so it must not say it is")
		assert.False(t, fx.Snapshot().Done)
	})

	t.Run("a stalled space that finally loads still finishes the run", func(t *testing.T) {
		// given: s2 has already gone Stalled
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "v2"}, true)
		loadedSpace(fx, "s1", "v1")
		fx.OnSpaceView("s2", "v2", false)
		fx.OnSpaceLoadStarted("s2", false)
		fx.clock.Advance(settleBound)
		require.Equal(t, pb.EventAccountRecovery_Stalled, fx.space(t, "s2").State)

		// when: the load finally lands
		fx.OnSpaceLoaded("s2", nil, false)

		// then
		fin := fx.finished(t)
		require.NotNil(t, fin)
		assert.True(t, fin.ViewsConfirmed)
		assert.Equal(t, int32(3), fin.SpacesLoaded)
	})

	t.Run("progress postpones the verdict", func(t *testing.T) {
		// given: a run that keeps making progress just under the bound
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)
		loadedSpace(fx, "s1", "v1")

		// when
		fx.clock.Advance(settleBound - time.Second)
		loadedSpace(fx, "s2", "v2")
		fx.clock.Advance(settleBound - time.Second)

		// then: the bound restarted, so nothing has been decided yet
		assert.Nil(t, fx.finished(t))

		// and when the run finally goes quiet
		fx.clock.Advance(settleBound)
		assert.NotNil(t, fx.finished(t))
	})
}

// TestTracker_OptimisticIsNotAVerdict pins that the on-disk fast path may show
// a space as Loaded without the run claiming the account is recovered. The
// loader shows Loaded before it has a result — the space is on disk and was Ok
// last session, so hiding it would be wrong — but it still runs the build, and
// on a warm start EVERY space takes that path. Finishing on it meant Finished
// fired before a single build had reported, and because the run goes silent at
// Finished, a build that then failed could never be reported at all.
func TestTracker_OptimisticIsNotAVerdict(t *testing.T) {
	t.Run("an optimistic space alone does not finish the run", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1"}, true)
		fx.OnSpaceView("s1", "v1", false)

		// when: shown Loaded off the disk, no build result yet
		fx.OnSpaceLoadStarted("s1", true)

		// then
		assert.Equal(t, pb.EventAccountRecovery_Loaded, fx.space(t, "s1").State, "the client must still see it")
		assert.Nil(t, fx.finished(t), "no build has reported, so nothing may be claimed")
	})

	t.Run("a failing build after the optimistic show is still reported", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1"}, true)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceLoadStarted("s1", true)

		// when: the build the loader still ran fails for real
		fx.OnSpaceLoaded("s1", errors.New("boom"), false)

		// then: the space regresses and the verdict counts it as failed
		assert.Equal(t, pb.EventAccountRecovery_Error, fx.space(t, "s1").State)
		fin := fx.finished(t)
		require.NotNil(t, fin)
		assert.Equal(t, int32(1), fin.SpacesFailed)
		assert.Equal(t, int32(1), fin.SpacesLoaded, "the tech space")
	})

	t.Run("a build that never reports still lets the run finish", func(t *testing.T) {
		// given: the space is usable off disk, but its build has gone quiet
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1"}, true)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceLoadStarted("s1", true)

		// when
		fx.clock.Advance(settleBound)

		// then: it works, so there is nothing to stall about
		fin := fx.finished(t)
		require.NotNil(t, fin)
		assert.True(t, fin.ViewsConfirmed)
		assert.Equal(t, int32(2), fin.SpacesLoaded)
		assert.Equal(t, pb.EventAccountRecovery_Loaded, fx.space(t, "s1").State)
	})
}

// TestTracker_RemovedStaysRemoved pins that a space the client was told to drop
// cannot come back through a producer event that was already in flight. Every
// producer creates its space entry on demand, so a load or pull result landing
// after the deletion resurrected it: the client saw a space it had dropped,
// spacesTotal grew, and anything short of a verdict held Finished open again.
func TestTracker_RemovedStaysRemoved(t *testing.T) {
	t.Run("a late load result does not resurrect a deleted space", func(t *testing.T) {
		// given: s1 is deleted while loading, with s2 keeping the run open
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "v2"}, true)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceLoadStarted("s1", false)
		fx.OnSpaceView("s2", "v2", false)
		fx.OnSpaceLoadStarted("s2", false)
		fx.OnSpaceView("s1", "v1", true)
		require.False(t, fx.hasSpace("s1"))
		total := fx.Snapshot().SpacesTotal

		// when: the build already in flight reports
		fx.OnSpaceLoaded("s1", nil, false)

		// then
		assert.False(t, fx.hasSpace("s1"), "the client was told to drop it")
		assert.Equal(t, total, fx.Snapshot().SpacesTotal)
	})

	t.Run("a late pull event does not resurrect it either", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "v2"}, true)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceView("s2", "v2", false)
		fx.OnSpaceLoadStarted("s2", false)
		fx.OnSpaceView("s1", "v1", true)
		total := fx.Snapshot().SpacesTotal

		// when
		fx.ObservePullEvent(commonspace.PullEvent{SpaceId: "s1", Kind: commonspace.PullEventWaiting})

		// then
		assert.False(t, fx.hasSpace("s1"))
		assert.Equal(t, total, fx.Snapshot().SpacesTotal)
	})

	t.Run("discovery brings a space back", func(t *testing.T) {
		// given: deleted, then the SpaceView subscription reports it present
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.ready(t)
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceView("s1", "v1", true)
		require.False(t, fx.hasSpace("s1"))

		// when: discovery is authoritative
		fx.OnSpaceView("s1", "v1", false)

		// then
		assert.True(t, fx.hasSpace("s1"))
	})
}
