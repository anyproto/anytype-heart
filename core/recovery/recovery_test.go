package recovery

import (
	"context"
	"errors"
	"testing"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/secureservice/handshake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
)

func TestTracker_Begin(t *testing.T) {
	t.Run("snapshot is idle before the first begin", func(t *testing.T) {
		// given
		tr := newTracker(&fakeClock{now: fixtureEpoch}, coalesceWindow)

		// then
		assert.Equal(t, IdleSnapshot(), tr.Snapshot())
	})

	t.Run("started is id 1 and carries the mode", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)

		// when
		fx.init(t)

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 1)
		want := &pb.EventAccountRecoveryUpdate{
			RunId:       ups[0].RunId,
			Id:          1,
			TimestampMs: fixtureEpoch.UnixMilli(),
			Payload: &pb.EventAccountRecoveryUpdatePayloadOfStarted{Started: &pb.EventAccountRecoveryStarted{
				Mode: pb.EventAccountRecovery_ColdRecovery,
			}},
		}
		assert.Equal(t, want, ups[0])
		assert.NotEmpty(t, ups[0].RunId)
		snap := fx.Snapshot()
		assert.Equal(t, int64(1), snap.LastEventId)
		assert.Equal(t, pb.EventAccountRecovery_LookingForPeers, snap.Phase)
		assert.False(t, snap.Done)
	})

	t.Run("init without begin still opens a warm run", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		tr := newTracker(fx.clock, coalesceWindow)
		fx.Tracker = tr

		// when
		fx.init(t)

		// then
		snap := tr.Snapshot()
		require.NotNil(t, snap)
		assert.Equal(t, pb.EventAccountRecovery_WarmStart, snap.Mode)
	})

	t.Run("a second begin resets ids and changes the run id", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		first := fx.Snapshot()
		fx.Fail(errors.New("boom"))
		require.True(t, fx.Snapshot().Done)

		// when
		fx.Begin(Run{Mode: pb.EventAccountRecovery_WarmStart, Sender: fx.sender})
		fx.init(t)

		// then
		second := fx.Snapshot()
		assert.NotEqual(t, first.RunId, second.RunId)
		assert.Equal(t, int64(1), second.LastEventId)
		assert.Equal(t, pb.EventAccountRecovery_WarmStart, second.Mode)
		assert.False(t, second.Done)
		assert.Nil(t, second.Error)
		last := fx.lastUpdate(t)
		assert.Equal(t, second.RunId, last.RunId)
		assert.Equal(t, int64(1), last.Id)
	})
}

func TestTracker_Fail(t *testing.T) {
	t.Run("account-level verdict ends the run in Failed", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.clock.Advance(1500 * 1e6)
		err := errors.Join(errors.New("init tech space"), spacesyncproto.ErrSpaceIsDeleted)

		// when
		fx.Fail(err)

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 2)
		want := &pb.EventAccountRecoveryPhaseChanged{
			Phase:                   pb.EventAccountRecovery_Failed,
			FromPhase:               pb.EventAccountRecovery_LookingForPeers,
			PreviousPhaseDurationMs: 1500,
			Error: &pb.EventAccountRecoveryErrorInfo{
				Class:        pb.EventAccountRecovery_AccountDeleted,
				DebugMessage: err.Error(),
			},
		}
		assert.Equal(t, int64(2), ups[1].Id)
		assert.Equal(t, want, ups[1].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged).PhaseChanged)
		snap := fx.Snapshot()
		assert.True(t, snap.Done)
		assert.Equal(t, pb.EventAccountRecovery_Failed, snap.Phase)
		assert.Equal(t, want.Error, snap.Error)
		assert.Equal(t, int64(2), snap.LastEventId)
	})

	t.Run("fail before init publishes started first", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)

		// when
		fx.Fail(handshake.ErrIncompatibleVersion)

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 2)
		_, isStarted := ups[0].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfStarted)
		assert.True(t, isStarted)
		changed := ups[1].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged).PhaseChanged
		assert.Equal(t, pb.EventAccountRecovery_IncompatibleVersion, changed.Error.Class)
		assert.False(t, changed.Error.Retryable)
	})

	t.Run("fail after close still publishes the terminal", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		require.NoError(t, fx.Close(context.Background()))

		// when
		fx.Fail(errors.New("can't run service"))

		// then
		last := fx.lastUpdate(t)
		changed := last.Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged).PhaseChanged
		assert.Equal(t, pb.EventAccountRecovery_Failed, changed.Phase)
		assert.Equal(t, pb.EventAccountRecovery_Unexpected, changed.Error.Class)
		assert.True(t, changed.Error.Retryable)
		snap := fx.Snapshot()
		assert.True(t, snap.Done, "the verdict revives the closed run in the snapshot")
		assert.Equal(t, pb.EventAccountRecovery_Failed, snap.Phase)
	})

	t.Run("a cancelled start is not a failure: no verdict, and the run reads idle", func(t *testing.T) {
		// given: the app's unwind closed the tracker before Fail, as it does
		// when the cancel struck a component past the tracker's Init
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		require.NoError(t, fx.Close(context.Background()))

		// when
		fx.Fail(errors.Join(errors.New("can't run service 'client.space'"), context.Canceled))

		// then
		assert.Len(t, fx.sender.updates(), 1, "only Started; no terminal for a cancel")
		assert.Equal(t, IdleSnapshot(), fx.Snapshot())
	})

	t.Run("a cancel that struck before the tracker was closed still ends the run", func(t *testing.T) {
		// given: a start that failed before app.Start's unwind reached the
		// tracker, so nothing called Close
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)

		// when
		fx.Fail(context.Canceled)

		// then
		assert.Empty(t, fx.sender.updates(), "Started is not published for a start that never began")
		assert.Equal(t, IdleSnapshot(), fx.Snapshot())
		assert.Equal(t, 0, fx.clock.pendingTimers())
	})

	t.Run("fail is idempotent", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.Fail(errors.New("first"))

		// when
		fx.Fail(errors.New("second"))

		// then
		assert.Len(t, fx.sender.updates(), 2)
		assert.Contains(t, fx.Snapshot().Error.DebugMessage, "first")
	})

	t.Run("fail before begin is a no-op", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		tr := newTracker(fx.clock, coalesceWindow)

		// when
		tr.Fail(errors.New("boom"))

		// then
		assert.Equal(t, IdleSnapshot(), tr.Snapshot())
	})
}

func TestTracker_Snapshot(t *testing.T) {
	t.Run("idle before any run: empty runId, NotStarted, nothing else, no error path", func(t *testing.T) {
		// given
		tr := newTracker(&fakeClock{now: fixtureEpoch}, coalesceWindow)

		// when
		got := tr.Snapshot()

		// then
		want := &pb.EventAccountRecoverySnapshot{
			RunId:      "",
			Phase:      pb.EventAccountRecovery_NotStarted,
			Mode:       pb.EventAccountRecovery_ModeUnknown,
			LocalPeers: pb.EventAccountRecovery_NoLocalPeers,
		}
		assert.Equal(t, want, got)
		assert.Equal(t, int64(0), got.LastEventId)
		assert.False(t, got.Done)
		assert.Empty(t, got.Peers)
		assert.Empty(t, got.Spaces)
		assert.Equal(t, int64(0), got.StartedAtMs, "no fake epoch leaks into the idle answer")
	})

	t.Run("becomes a real snapshot at Begin, before Init", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)

		// when
		got := fx.Snapshot()

		// then
		assert.NotEmpty(t, got.RunId)
		assert.Equal(t, pb.EventAccountRecovery_LookingForPeers, got.Phase)
		assert.Equal(t, pb.EventAccountRecovery_ColdRecovery, got.Mode)
		assert.Equal(t, int64(0), got.LastEventId, "Started is published from Init")
		assert.Equal(t, fixtureEpoch.UnixMilli(), got.StartedAtMs)
	})

	t.Run("a terminal, closed run keeps reporting itself rather than idle", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.Fail(errors.New("boom"))
		require.NoError(t, fx.Close(context.Background()))
		runId := fx.Snapshot().RunId

		// when
		got := fx.Snapshot()

		// then
		assert.Equal(t, runId, got.RunId)
		assert.True(t, got.Done)
		assert.Equal(t, pb.EventAccountRecovery_Failed, got.Phase)
		assert.Equal(t, int64(2), got.LastEventId)
		assert.NotEqual(t, IdleSnapshot(), got)
	})

	t.Run("the next Begin starts a new run, never idle in between", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.Fail(errors.New("boom"))
		first := fx.Snapshot().RunId

		// when
		fx.Begin(Run{Mode: pb.EventAccountRecovery_ColdRecovery, Sender: fx.sender})

		// then
		got := fx.Snapshot()
		assert.NotEqual(t, first, got.RunId)
		assert.NotEmpty(t, got.RunId)
		assert.False(t, got.Done)
		assert.Equal(t, pb.EventAccountRecovery_LookingForPeers, got.Phase)
	})

	t.Run("a run closed before its verdict is over: it reads idle, not as a live run", func(t *testing.T) {
		// given: the account was stopped mid-recovery
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.dialStarted("node1", 1)
		require.False(t, fx.Snapshot().Done)

		// when
		require.NoError(t, fx.Close(context.Background()))

		// then
		assert.Equal(t, IdleSnapshot(), fx.Snapshot())

		// and the next Begin is a fresh run again
		fx.Begin(Run{Mode: pb.EventAccountRecovery_WarmStart, Sender: fx.sender})
		got := fx.Snapshot()
		assert.NotEmpty(t, got.RunId)
		assert.Equal(t, pb.EventAccountRecovery_WarmStart, got.Mode)
	})
}

func TestTracker_Close(t *testing.T) {
	t.Run("close drops the pending window and silences a run without a verdict", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.mu.Lock()
		fx.markLocked(dialFailedPayload("p1", 1), &coalesceKey{kind: "dialFailed", id: "p1"})
		fx.markLocked(dialFailedPayload("p1", 2), &coalesceKey{kind: "dialFailed", id: "p1"})
		fx.mu.Unlock()
		require.Equal(t, 1, fx.clock.pendingTimers())

		// when
		require.NoError(t, fx.Close(context.Background()))

		// then: the run is over without a verdict, so the level the window
		// held is not sent — the snapshot no longer reports the run it
		// belongs to — and nothing publishes after
		assert.Len(t, fx.sender.updates(), 1, "only Started")
		assert.Equal(t, 0, fx.clock.pendingTimers())
		assert.Equal(t, IdleSnapshot(), fx.Snapshot(), "closed without a verdict: the run is over")
		fx.mu.Lock()
		fx.markLocked(dialFailedPayload("p1", 3), nil)
		fx.mu.Unlock()
		assert.Len(t, fx.sender.updates(), 1)
	})

	t.Run("a verdict after close carries the window it held", func(t *testing.T) {
		// given: a failed app.Start closes the tracker before Fail reports
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.mu.Lock()
		fx.markLocked(dialFailedPayload("p1", 1), &coalesceKey{kind: "dialFailed", id: "p1"})
		fx.mu.Unlock()
		require.NoError(t, fx.Close(context.Background()))

		// when
		fx.Fail(errors.New("can't run service"))

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 3)
		assert.Equal(t, int32(1), ups[1].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfDialFailed).DialFailed.Attempt)
		assert.Equal(t, pb.EventAccountRecovery_Failed, ups[2].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged).PhaseChanged.Phase)
		assert.True(t, fx.Snapshot().Done)
	})
}

func TestTracker_SessionHook(t *testing.T) {
	t.Run("a new session receives the snapshot with the last id", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.Fail(errors.New("boom"))

		// when
		fx.hooks.RunHooks(session.NewContext(session.WithSession("token")))

		// then
		events := fx.sender.sessions["token"]
		require.Len(t, events, 1)
		ups := updatesOf(events[0])
		require.Len(t, ups, 1)
		assert.Equal(t, int64(2), ups[0].Id)
		want := fx.Snapshot()
		assert.Equal(t, want, ups[0].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfSnapshot).Snapshot)
	})

	t.Run("no snapshot before started", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)

		// when
		require.NoError(t, fx.sendSnapshotToSession(session.NewContext(session.WithSession("token"))))

		// then
		assert.Empty(t, fx.sender.sessions["token"])
	})

	t.Run("no snapshot for a run that ended without a verdict", func(t *testing.T) {
		// given: a cancelled start, closed by the app's unwind
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		require.NoError(t, fx.Close(context.Background()))
		fx.Fail(context.Canceled)

		// when: a session attaches later
		fx.hooks.RunHooks(session.NewContext(session.WithSession("token")))

		// then: it is not told about a run that is over
		assert.Empty(t, fx.sender.sessions["token"])
	})
}

func TestTracker_PanicContainment(t *testing.T) {
	t.Run("a panicking sender does not escape the entry point", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.sender.panicOnce = true

		// when / then
		assert.NotPanics(t, func() { fx.init(t) })
	})
}

func dialFailedPayload(peerId string, attempt int32) pb.IsEventAccountRecoveryUpdatePayload {
	return &pb.EventAccountRecoveryUpdatePayloadOfDialFailed{DialFailed: &pb.EventAccountRecoveryDialFailed{
		PeerId:  peerId,
		Attempt: attempt,
	}}
}
