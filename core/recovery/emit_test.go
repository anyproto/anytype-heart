package recovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

func TestTracker_Coalescing(t *testing.T) {
	key := &coalesceKey{kind: "dialFailed", id: "p1"}
	mark := func(fx *fixture, payload pb.IsEventAccountRecoveryUpdatePayload, key *coalesceKey) {
		fx.mu.Lock()
		defer fx.mu.Unlock()
		fx.markLocked(payload, key)
	}

	t.Run("the first keyed event of a burst publishes at once", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.clock.Advance(time.Second)

		// when
		mark(fx, dialFailedPayload("p1", 1), key)

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 2)
		assert.Equal(t, int64(2), ups[1].Id)
		assert.Equal(t, 0, fx.clock.pendingTimers())
	})

	t.Run("a burst inside the window collapses to one trailing level", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t) // Started opens the window at t=0

		// when
		for i := int32(1); i <= 5; i++ {
			fx.clock.Advance(20 * time.Millisecond)
			mark(fx, dialFailedPayload("p1", i), key)
		}
		require.Len(t, fx.sender.updates(), 1, "nothing leaves before the trailing edge")
		fx.clock.Advance(coalesceWindow)

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 2)
		got := ups[1].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfDialFailed).DialFailed
		want := &pb.EventAccountRecoveryDialFailed{PeerId: "p1", Attempt: 5}
		assert.Equal(t, want, got)
		assert.Equal(t, int64(2), ups[1].Id)
		assert.Equal(t, 0, fx.clock.pendingTimers())
	})

	t.Run("an edge flushes the pending window first, in key order, as one event", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.clock.Advance(10 * time.Millisecond)
		mark(fx, dialFailedPayload("p2", 1), &coalesceKey{kind: "dialFailed", id: "p2"})
		mark(fx, dialFailedPayload("p1", 1), key)
		mark(fx, dialFailedPayload("p2", 2), &coalesceKey{kind: "dialFailed", id: "p2"})

		// when
		mark(fx, &pb.EventAccountRecoveryUpdatePayloadOfAccountReady{AccountReady: &pb.EventAccountRecoveryAccountReady{}}, nil)

		// then
		require.Len(t, fx.sender.broadcasts, 2)
		ups := updatesOf(fx.sender.broadcasts[1])
		require.Len(t, ups, 3)
		assert.Equal(t, []int64{2, 3, 4}, []int64{ups[0].Id, ups[1].Id, ups[2].Id})
		assert.Equal(t, "p2", ups[0].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfDialFailed).DialFailed.PeerId)
		assert.Equal(t, int32(2), ups[0].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfDialFailed).DialFailed.Attempt)
		assert.Equal(t, "p1", ups[1].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfDialFailed).DialFailed.PeerId)
		_, isReady := ups[2].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfAccountReady)
		assert.True(t, isReady)
		// the timer armed for the burst is now moot
		fx.clock.Advance(coalesceWindow)
		assert.Len(t, fx.sender.broadcasts, 2)
	})

	t.Run("the snapshot already reflects a pending level and re-applies idempotently", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.clock.Advance(10 * time.Millisecond)
		mark(fx, dialFailedPayload("p1", 1), key)

		// when
		snap := fx.Snapshot()

		// then
		assert.Equal(t, int64(1), snap.LastEventId, "pending events have no id yet")
		fx.clock.Advance(coalesceWindow)
		assert.Equal(t, int64(2), fx.lastUpdate(t).Id)
	})
}
