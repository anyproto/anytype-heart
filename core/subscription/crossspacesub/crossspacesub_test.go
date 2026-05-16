package crossspacesub

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/pb"
)

func newBareCrossSpaceSub(t *testing.T, ms subscriptionservice.Service, pending ...string) *crossSpaceSubscription {
	s := &crossSpaceSubscription{
		subId:                 "cs",
		request:               subscriptionservice.SubscribeRequest{},
		subscriptionService:   ms,
		perSpaceSubscriptions: map[string]string{},
		inflightSpaceIds:      map[string]uint64{},
		pendingSpaceIds:       map[string]struct{}{},
		totalCounts:           map[string]int64{},
		queue:                 mb.New[*pb.EventMessage](0),
	}
	for _, p := range pending {
		s.pendingSpaceIds[p] = struct{}{}
	}
	t.Cleanup(func() { _ = s.queue.Close() })
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
