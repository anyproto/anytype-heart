package event

import (
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
)

// seqEvent encodes i into an event via a SubscriptionAdd id, so a fake send can
// recover the delivery order.
func seqEvent(i int) *pb.Event {
	return &pb.Event{Messages: []*pb.EventMessage{{
		Value: &pb.EventMessageValueOfSubscriptionAdd{
			SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: strconv.Itoa(i)},
		},
	}}}
}

func seqOf(e *pb.Event) int {
	id := e.Messages[0].GetSubscriptionAdd().Id
	n, _ := strconv.Atoi(id)
	return n
}

func TestSessionSender_deliversInOrder(t *testing.T) {
	const n = 200
	var mu sync.Mutex
	var got []int
	send := func(e *pb.Event) error {
		mu.Lock()
		got = append(got, seqOf(e))
		mu.Unlock()
		return nil
	}
	s := newSessionSender(send, func() {}, maxSessionQueueMessages)
	defer s.close()

	for i := 0; i < n; i++ {
		s.enqueue(seqEvent(i))
	}
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(got) == n },
		2*time.Second, time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	for i := 0; i < n; i++ {
		require.Equal(t, i, got[i], "events must be delivered in enqueue order")
	}
}

func TestSessionSender_enqueueIsNonBlockingAndOverflowCloses(t *testing.T) {
	block := make(chan struct{})
	var closed atomic.Bool
	send := func(e *pb.Event) error { <-block; return nil } // drain stalls on the first send
	s := newSessionSender(send, func() { closed.Store(true) }, 5) // tiny bound
	defer func() { close(block); s.close() }()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ { // far past the bound of 5
			s.enqueue(seqEvent(i))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("enqueue blocked on a stalled client")
	}
	require.Eventually(t, closed.Load, 2*time.Second, time.Millisecond,
		"overflow must close the session")
}

func TestSessionSender_sendErrorClosesAndStops(t *testing.T) {
	var calls atomic.Int32
	var closed atomic.Bool
	send := func(e *pb.Event) error {
		calls.Add(1)
		return errors.New("stream broken")
	}
	s := newSessionSender(send, func() { closed.Store(true) }, maxSessionQueueMessages)
	defer s.close()

	for i := 0; i < 50; i++ { // many: if the drain didn't stop it would send all 50
		s.enqueue(seqEvent(i))
	}
	require.Eventually(t, closed.Load, 2*time.Second, time.Millisecond, "send error must close")
	require.Eventually(t, func() bool { return calls.Load() >= 1 }, time.Second, time.Millisecond)
	time.Sleep(30 * time.Millisecond) // let any (buggy) further sends happen
	require.LessOrEqual(t, calls.Load(), int32(1), "drain must stop after the first send error")
}

func TestSessionSender_closeStopsDrainGoroutine(t *testing.T) {
	var calls atomic.Int32
	send := func(e *pb.Event) error { calls.Add(1); return nil }
	s := newSessionSender(send, func() {}, maxSessionQueueMessages)
	s.close()
	select {
	case <-s.done:
	case <-time.After(time.Second):
		t.Fatal("drain goroutine did not exit after close()")
	}
	// enqueue after close is dropped, never sent
	s.enqueue(seqEvent(0))
	time.Sleep(20 * time.Millisecond)
	require.Equal(t, int32(0), calls.Load())
}
