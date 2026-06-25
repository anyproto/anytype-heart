package crossspacesub

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/pb"
)

func newBareCrossSpaceSub(t *testing.T, ms subscriptionservice.Service, pending ...string) *crossSpaceSubscription {
	ctx, ctxCancel := context.WithCancel(context.Background())
	s := &crossSpaceSubscription{
		subId:                 "cs",
		request:               subscriptionservice.SubscribeRequest{},
		subscriptionService:   ms,
		ctx:                   ctx,
		ctxCancel:             ctxCancel,
		perSpaceSubscriptions: map[string]string{},
		inflightSpaceIds:      map[string]uint64{},
		pendingSpaceIds:       map[string]struct{}{},
		totalCounts:           map[string]int64{},
		activeInternalSubs:    map[string]struct{}{},
		queue:                 mb.New[*pb.EventMessage](0),
		clk:                   realClock{},
	}
	for _, p := range pending {
		s.pendingSpaceIds[p] = struct{}{}
	}
	t.Cleanup(func() {
		ctxCancel()
		_ = s.queue.Close()
	})
	return s
}

// PromotePending must not hold s.lock while subscribe() (which re-enters the
// subscription service that itself can call back into this subscription) runs.
// Holding it is the GO-7288 ABBA deadlock: subscription-service lock <-> this
// lock.
func TestPromotePending_doesNotHoldLockDuringSubscribe(t *testing.T) {
	ms := mock_subscription.NewMockService(t)
	s := newBareCrossSpaceSub(t, ms, "space1")

	var lockHeldDuringSubscribe bool
	ms.EXPECT().Search(mock.Anything).RunAndReturn(
		func(subscriptionservice.SubscribeRequest) (*subscriptionservice.SubscribeResponse, error) {
			lockFree := make(chan struct{})
			go func() {
				s.AddPending("other") // needs s.lock
				close(lockFree)
			}()
			select {
			case <-lockFree:
				lockHeldDuringSubscribe = false
			case <-time.After(time.Second):
				lockHeldDuringSubscribe = true
			}
			return &subscriptionservice.SubscribeResponse{
				SubId:    "sp1",
				Counters: &pb.EventObjectSubscriptionCounters{},
			}, nil
		})

	require.NoError(t, s.PromotePending("space1"))

	assert.False(t, lockHeldDuringSubscribe,
		"PromotePending held s.lock across subscribe(): ABBA deadlock risk (GO-7288)")
	assert.Equal(t, "sp1", s.perSpaceSubscriptions["space1"])
}

// A RemoveSpace + re-AddSpace that straddles an in-flight subscribe must not
// leak a subscription: the superseded in-flight subscribe must roll back its
// freshly created sub instead of overwriting the live one.
func TestEnsureSpaceSubscribed_removeDuringSubscribeRollsBack(t *testing.T) {
	ms := mock_subscription.NewMockService(t)
	s := newBareCrossSpaceSub(t, ms, "space1")

	var searchCalls int32
	entered := make(chan struct{})
	release := make(chan struct{})
	ms.EXPECT().Search(mock.Anything).RunAndReturn(
		func(subscriptionservice.SubscribeRequest) (*subscriptionservice.SubscribeResponse, error) {
			n := atomic.AddInt32(&searchCalls, 1)
			if n == 1 { // PromotePending's subscribe: block mid-flight
				close(entered)
				<-release
				return &subscriptionservice.SubscribeResponse{
					SubId: "sub-A", Counters: &pb.EventObjectSubscriptionCounters{},
				}, nil
			}
			return &subscriptionservice.SubscribeResponse{
				SubId: "sub-B", Counters: &pb.EventObjectSubscriptionCounters{},
			}, nil
		})

	var rolledBack []string
	ms.EXPECT().UnsubscribeAndReturnIds("space1", mock.Anything).RunAndReturn(
		func(_ string, subId string) ([]string, error) {
			rolledBack = append(rolledBack, subId)
			return nil, nil
		}).Maybe()

	promoteDone := make(chan error, 1)
	go func() { promoteDone <- s.PromotePending("space1") }()

	<-entered // PromotePending is now mid-subscribe, reservation held
	s.RemoveSpace("space1")
	require.NoError(t, s.AddSpace("space1")) // re-adds: subscribes sub-B
	close(release)                           // let PromotePending's subscribe return sub-A
	require.NoError(t, <-promoteDone)

	assert.Equal(t, "sub-B", s.perSpaceSubscriptions["space1"],
		"the live (re-added) subscription must be the tracked one")
	assert.Equal(t, []string{"sub-A"}, rolledBack,
		"the superseded in-flight subscription must be rolled back, not leaked")
}

// Concurrent PromotePending for the same space must subscribe exactly once.
func TestPromotePending_concurrentSameSpaceSubscribesOnce(t *testing.T) {
	ms := mock_subscription.NewMockService(t)
	s := newBareCrossSpaceSub(t, ms, "space1")

	var calls int32
	ms.EXPECT().Search(mock.Anything).RunAndReturn(
		func(subscriptionservice.SubscribeRequest) (*subscriptionservice.SubscribeResponse, error) {
			atomic.AddInt32(&calls, 1)
			time.Sleep(20 * time.Millisecond) // widen the race window
			return &subscriptionservice.SubscribeResponse{
				SubId:    "sp1",
				Counters: &pb.EventObjectSubscriptionCounters{},
			}, nil
		}).Maybe()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, s.PromotePending("space1"))
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"space promoted concurrently must be subscribed exactly once")
	assert.Equal(t, "sp1", s.perSpaceSubscriptions["space1"])
}

// A RemoveSpace racing PromotePending must never leave the space subscribed:
// the promote decision (consuming the pending entry) and the reservation claim
// must be atomic, otherwise a RemoveSpace slipping between them finds nothing
// to cancel and the promote resurrects a just-removed space (ghost
// subscription). Exercises many interleavings; run with -race.
func TestPromotePending_removeRacingPromoteNeverLeavesGhost(t *testing.T) {
	for i := 0; i < 500; i++ {
		ms := mock_subscription.NewMockService(t)
		s := newBareCrossSpaceSub(t, ms, "space1")

		var (
			trackLock sync.Mutex
			created   []string
			removed   []string
		)
		ms.EXPECT().Search(mock.Anything).RunAndReturn(
			func(subscriptionservice.SubscribeRequest) (*subscriptionservice.SubscribeResponse, error) {
				subId := fmt.Sprintf("sub-%d", i)
				trackLock.Lock()
				created = append(created, subId)
				trackLock.Unlock()
				return &subscriptionservice.SubscribeResponse{
					SubId: subId, Counters: &pb.EventObjectSubscriptionCounters{},
				}, nil
			}).Maybe()
		ms.EXPECT().UnsubscribeAndReturnIds("space1", mock.Anything).RunAndReturn(
			func(_ string, subId string) ([]string, error) {
				trackLock.Lock()
				removed = append(removed, subId)
				trackLock.Unlock()
				return nil, nil
			}).Maybe()

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = s.PromotePending("space1")
		}()
		go func() {
			defer wg.Done()
			s.RemoveSpace("space1")
		}()
		wg.Wait()

		s.lock.Lock()
		subId, ghost := s.perSpaceSubscriptions["space1"]
		s.lock.Unlock()
		// Whatever the interleaving, the space must end unsubscribed: either
		// RemoveSpace ran last and cleaned up, the promote lost its
		// reservation and rolled back, or the promote never started because
		// the pending entry was already consumed.
		require.Falsef(t, ghost,
			"iteration %d: space1 still subscribed (%s) after RemoveSpace", i, subId)
		trackLock.Lock()
		require.ElementsMatchf(t, created, removed,
			"iteration %d: every created per-space sub must be unsubscribed", i)
		trackLock.Unlock()
	}
}

// When an in-flight subscribe loses its reservation, the rollback must emit
// SubscriptionRemove (and a zeroing counters event) for the ids the rolled-back
// sub had already delivered via its async-init events — otherwise clients keep
// ghost records of a space that was removed.
func TestEnsureSpaceSubscribed_rollbackEmitsRemovalEvents(t *testing.T) {
	ms := mock_subscription.NewMockService(t)
	s := newBareCrossSpaceSub(t, ms, "space1")

	entered := make(chan struct{})
	release := make(chan struct{})
	ms.EXPECT().Search(mock.Anything).RunAndReturn(
		func(subscriptionservice.SubscribeRequest) (*subscriptionservice.SubscribeResponse, error) {
			close(entered)
			<-release
			return &subscriptionservice.SubscribeResponse{
				SubId: "sub-A", Counters: &pb.EventObjectSubscriptionCounters{},
			}, nil
		})
	ms.EXPECT().UnsubscribeAndReturnIds("space1", "sub-A").Return([]string{"obj1", "obj2"}, nil)

	promoteDone := make(chan error, 1)
	go func() { promoteDone <- s.PromotePending("space1") }()

	<-entered
	s.RemoveSpace("space1") // cancels the reservation; nothing subscribed yet
	close(release)
	require.NoError(t, <-promoteDone)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msgs, err := s.queue.NewCond().WithMin(3).Wait(ctx)
	require.NoError(t, err, "rollback must emit removal events for the rolled-back sub's ids")

	var removedIds []string
	var counterSubIds []string
	for _, msg := range msgs {
		if rm := msg.GetSubscriptionRemove(); rm != nil {
			removedIds = append(removedIds, rm.Id)
			assert.Equal(t, "cs", rm.SubId)
		}
		if c := msg.GetSubscriptionCounters(); c != nil {
			counterSubIds = append(counterSubIds, c.SubId)
			assert.Zero(t, c.Total)
		}
	}
	assert.ElementsMatch(t, []string{"obj1", "obj2"}, removedIds)
	// the cross-space subId: a counters event keyed by the internal subId
	// would make patchEvent re-insert the per-space total into totalCounts
	assert.Equal(t, []string{"cs"}, counterSubIds)
}

// close() racing an in-flight subscribe must not finalize the subscription:
// nothing would ever unsubscribe it (close already ran), leaving a live
// per-space sub pushing into a closed queue forever. The finalize step must
// detect the closed subscription and roll back instead.
func TestEnsureSpaceSubscribed_closeDuringSubscribeRollsBack(t *testing.T) {
	ms := mock_subscription.NewMockService(t)
	s := newBareCrossSpaceSub(t, ms, "space1")

	entered := make(chan struct{})
	release := make(chan struct{})
	ms.EXPECT().Search(mock.Anything).RunAndReturn(
		func(subscriptionservice.SubscribeRequest) (*subscriptionservice.SubscribeResponse, error) {
			close(entered)
			<-release
			return &subscriptionservice.SubscribeResponse{
				SubId: "sub-A", Counters: &pb.EventObjectSubscriptionCounters{},
			}, nil
		})

	var rolledBack []string
	ms.EXPECT().UnsubscribeAndReturnIds("space1", mock.Anything).RunAndReturn(
		func(_ string, subId string) ([]string, error) {
			rolledBack = append(rolledBack, subId)
			return nil, nil
		}).Maybe()

	promoteDone := make(chan error, 1)
	go func() { promoteDone <- s.PromotePending("space1") }()

	<-entered
	require.NoError(t, s.close())
	close(release)
	require.NoError(t, <-promoteDone)

	s.lock.Lock()
	_, subscribed := s.perSpaceSubscriptions["space1"]
	s.lock.Unlock()
	assert.False(t, subscribed,
		"a subscribe finalizing after close() must not be tracked: nothing will ever unsubscribe it")
	assert.Equal(t, []string{"sub-A"}, rolledBack,
		"the per-space sub created during close must be rolled back, not leaked")
}

// The promote decision (consuming the pending entry) and the reservation
// claim must be one atomic critical section. If the lock is released between
// them, the space is observably in limbo — not pending, not reserved, not
// subscribed — and a RemoveSpace landing there finds nothing to cancel, so
// the promote then resurrects the just-removed space (ghost subscription).
// A prober contending on s.lock asserts the limbo state is never observable
// while a promote is in flight.
func TestPromotePending_decisionAndClaimAreAtomic(t *testing.T) {
	ms := mock_subscription.NewMockService(t)
	s := newBareCrossSpaceSub(t, ms, "space1")

	var subSeq atomic.Int64
	ms.EXPECT().Search(mock.Anything).RunAndReturn(
		func(subscriptionservice.SubscribeRequest) (*subscriptionservice.SubscribeResponse, error) {
			return &subscriptionservice.SubscribeResponse{
				SubId:    fmt.Sprintf("sub-%d", subSeq.Add(1)),
				Counters: &pb.EventObjectSubscriptionCounters{},
			}, nil
		}).Maybe()
	ms.EXPECT().UnsubscribeAndReturnIds("space1", mock.Anything).Return(nil, nil).Maybe()

	var (
		promoteInFlight atomic.Bool
		gapObserved     atomic.Bool
		stopProbers     atomic.Bool
		proberWg        sync.WaitGroup
	)
	// Several probers keep the mutex contended so one of them wins the lock
	// between the promote's critical sections (a single prober loses the
	// race to the promote's immediate re-lock essentially always).
	for p := 0; p < max(2, runtime.NumCPU()/2); p++ {
		proberWg.Add(1)
		go func() {
			defer proberWg.Done()
			for !stopProbers.Load() {
				s.lock.Lock()
				_, pending := s.pendingSpaceIds["space1"]
				_, inflight := s.inflightSpaceIds["space1"]
				_, subscribed := s.perSpaceSubscriptions["space1"]
				if promoteInFlight.Load() && !pending && !inflight && !subscribed {
					gapObserved.Store(true)
				}
				s.lock.Unlock()
			}
		}()
	}

	for i := 0; i < 20000 && !gapObserved.Load(); i++ {
		s.lock.Lock()
		s.pendingSpaceIds["space1"] = struct{}{}
		s.lock.Unlock()

		promoteInFlight.Store(true)
		require.NoError(t, s.PromotePending("space1"))
		promoteInFlight.Store(false)

		s.RemoveSpace("space1") // reset for the next round
	}
	stopProbers.Store(true)
	proberWg.Wait()

	assert.False(t, gapObserved.Load(),
		"promote was observable in limbo (not pending, not reserved, not subscribed): a RemoveSpace in that window cannot cancel it and the space gets resurrected")
}

func TestRunBroadcast_graceDelaysFirstFlush(t *testing.T) {
	ms := mock_subscription.NewMockService(t)
	s := newBareCrossSpaceSub(t, ms)
	fc := newFakeClock()
	s.clk = fc
	s.initialGrace = 200 * time.Millisecond
	s.window = 50 * time.Millisecond
	s.createdAt = fc.now()

	var mu sync.Mutex
	var broadcasts int
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Broadcast(mock.Anything).Run(func(*pb.Event) {
		mu.Lock()
		broadcasts++
		mu.Unlock()
	}).Maybe()
	s.eventSender = sender

	go s.runBroadcast()

	require.NoError(t, s.queue.Add(context.Background(), addMsg("a")))
	// run loop must arm the grace timer before we advance the clock
	require.Eventually(t, func() bool { return fc.numWaiters() >= 1 }, time.Second, time.Millisecond)
	mu.Lock()
	assert.Equal(t, 0, broadcasts, "no broadcast before grace deadline")
	mu.Unlock()

	fc.advance(200 * time.Millisecond)
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return broadcasts == 1 },
		time.Second, time.Millisecond, "exactly one broadcast after grace")
}

type fakeClock struct {
	mu      sync.Mutex
	t       time.Time
	waiters []fakeWaiter
}

type fakeWaiter struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1000, 0)} }

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) after(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.t
		return ch
	}
	c.waiters = append(c.waiters, fakeWaiter{at: c.t.Add(d), ch: ch})
	return ch
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.at.After(c.t) {
			w.ch <- c.t
		} else {
			kept = append(kept, w)
		}
	}
	c.waiters = kept
}

func (c *fakeClock) numWaiters() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}

func countersValue(m *pb.EventMessage) (int64, bool) {
	if c := m.GetSubscriptionCounters(); c != nil {
		return c.Total, true
	}
	return 0, false
}

// A per-space counter still queued when its space is removed must not resurrect
// the removed space's total. GO-7337.
func TestPatchEvent_removedSpaceCounterDoesNotResurrectTotal(t *testing.T) {
	ms := mock_subscription.NewMockService(t)
	s := newBareCrossSpaceSub(t, ms)
	s.perSpaceSubscriptions["space1"] = "sub-A"
	s.activeInternalSubs["sub-A"] = struct{}{}
	ms.EXPECT().UnsubscribeAndReturnIds("space1", "sub-A").Return([]string{"obj1", "obj2"}, nil)

	// 1) per-space A's initial counter is still queued (unpatched)
	queuedCounter := event.NewMessage("space1", &pb.EventMessageValueOfSubscriptionCounters{
		SubscriptionCounters: &pb.EventObjectSubscriptionCounters{SubId: "sub-A", Total: 2},
	})

	// 2) space removed: marks sub-A inactive and zeroes its total
	s.RemoveSpace("space1")

	// 3) the stale queued counter is now patched
	s.patchEvent(queuedCounter)

	total, ok := countersValue(queuedCounter)
	require.True(t, ok)
	assert.Equal(t, int64(0), total, "removed space's queued counter must not re-add its total")
	s.lock.Lock()
	assert.Equal(t, int64(0), s.getTotalCount())
	s.lock.Unlock()
}
