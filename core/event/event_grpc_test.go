//go:build !nogrpcserver && !_test
// +build !nogrpcserver,!_test

package event

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
)

// fakeStream implements service.ClientCommands_ListenSessionEventsServer; only
// Send and Context are exercised.
type fakeStream struct {
	grpc.ServerStream
	mu   sync.Mutex
	sent []*pb.Event
	send func(*pb.Event) error
}

func (f *fakeStream) Send(e *pb.Event) error {
	if f.send != nil {
		if err := f.send(e); err != nil {
			return err
		}
	}
	f.mu.Lock()
	f.sent = append(f.sent, e)
	f.mu.Unlock()
	return nil
}
func (f *fakeStream) Context() context.Context { return context.Background() }

var _ service.ClientCommands_ListenSessionEventsServer = (*fakeStream)(nil)

func nonEmptyEvent() *pb.Event {
	return &pb.Event{Messages: []*pb.EventMessage{{
		Value: &pb.EventMessageValueOfSubscriptionAdd{SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: "x"}},
	}}}
}

// A reconnect with the same token must stop the previous session's drain
// goroutine (close its sender), not leak it.
func TestSetSessionServer_overwriteClosesOldSender(t *testing.T) {
	es := NewGrpcSender()
	old := es.SetSessionServer("tok", &fakeStream{})
	es.SetSessionServer("tok", &fakeStream{}) // supersede
	require.Eventually(t, func() bool {
		return old.sender.queue.Add(context.Background(), nonEmptyEvent()) != nil // ErrClosed once closed
	}, time.Second, time.Millisecond, "old session's queue must be closed on overwrite")
}

// Overflow-triggered close while Broadcast holds the read lock must not deadlock
// (scheduleClose offloads the shutdownCh send to its own goroutine).
func TestBroadcast_overflowCloseDoesNotDeadlock(t *testing.T) {
	es := NewGrpcSender()
	block := make(chan struct{})
	defer close(block)
	// a stream whose Send blocks forever -> the queue fills -> enqueue overflows
	es.SetSessionServer("tok", &fakeStream{send: func(*pb.Event) error { <-block; return nil }})

	done := make(chan struct{})
	go func() {
		for i := 0; i < maxSessionQueueMessages+maxSendBatch+10; i++ {
			es.Broadcast(nonEmptyEvent()) // takes ServerMutex.RLock each call
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("Broadcast deadlocked on overflow-triggered close")
	}
	// the session is eventually torn down
	require.Eventually(t, func() bool {
		es.ServerMutex.RLock()
		defer es.ServerMutex.RUnlock()
		_, ok := es.Servers["tok"]
		return !ok
	}, 5*time.Second, time.Millisecond)
}

// Events broadcast to a healthy session arrive in order, exactly once.
func TestBroadcast_deliversInOrder(t *testing.T) {
	es := NewGrpcSender()
	fs := &fakeStream{}
	es.SetSessionServer("tok", fs)
	const n = 100
	for i := 0; i < n; i++ {
		es.Broadcast(seqEvent(i))
	}
	require.Eventually(t, func() bool {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return len(fs.sent) == n
	}, 2*time.Second, time.Millisecond)
	fs.mu.Lock()
	defer fs.mu.Unlock()
	for i := 0; i < n; i++ {
		require.Equal(t, i, seqOf(fs.sent[i]), "events must arrive in broadcast order")
	}
}

// The filtered send variants route to the right sessions.
func TestSendVariants_routing(t *testing.T) {
	es := NewGrpcSender()
	a, b := &fakeStream{}, &fakeStream{}
	es.SetSessionServer("a", a)
	es.SetSessionServer("b", b)

	es.SendToSession("a", seqEvent(1))                  // only a
	es.BroadcastToOtherSessions("a", seqEvent(2))       // only b (not the origin a)
	es.BroadcastExceptSessions(seqEvent(3), []string{"b"}) // only a

	countFor := func(fs *fakeStream) int {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return len(fs.sent)
	}
	require.Eventually(t, func() bool { return countFor(a) == 2 && countFor(b) == 1 },
		2*time.Second, time.Millisecond, "SendToSession+except hit a; BroadcastToOthers hit b")
}

// A handler that must not lose an event needs to wait for the caller's stream:
// the client opens ListenSessionEvents and calls the RPC as two independent
// requests, so the stream is often not registered yet when the handler runs.
// WaitForSession runs on this goroutine and the attach is delayed, so it must
// observe the absent session first — no scheduling hole that could let this pass
// via the already-attached fast path.
func TestWaitForSession_returnsWhenStreamAttachesLater(t *testing.T) {
	es := NewGrpcSender()
	const attachAfter = 150 * time.Millisecond
	go func() {
		time.Sleep(attachAfter)
		es.SetSessionServer("tok", &fakeStream{})
	}()

	start := time.Now()
	require.True(t, es.WaitForSession(context.Background(), "tok", 10*time.Second))
	require.GreaterOrEqual(t, time.Since(start), attachAfter, "must have waited for the attach, not fast-pathed")
}

// An already-attached session returns at once.
func TestWaitForSession_alreadyAttached(t *testing.T) {
	es := NewGrpcSender()
	es.SetSessionServer("tok", &fakeStream{})

	start := time.Now()
	require.True(t, es.WaitForSession(context.Background(), "tok", time.Minute))
	require.Less(t, time.Since(start), waitForSessionPollInterval, "must not sleep on the fast path")
}

// A client that never opens a stream falls through, bounded by the timeout.
func TestWaitForSession_timesOut(t *testing.T) {
	es := NewGrpcSender()
	const timeout = 150 * time.Millisecond

	start := time.Now()
	require.False(t, es.WaitForSession(context.Background(), "tok", timeout))
	elapsed := time.Since(start)
	require.GreaterOrEqual(t, elapsed, timeout)
	require.Less(t, elapsed, 5*time.Second, "must not outlive its own timeout")
}

// A caller that goes away (client disconnect) unblocks immediately.
func TestWaitForSession_canceledCaller(t *testing.T) {
	es := NewGrpcSender()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	require.False(t, es.WaitForSession(ctx, "tok", time.Minute))
	require.Less(t, time.Since(start), 5*time.Second, "a canceled caller must not wait out the timeout")
}

// Another session attaching is not this session's stream.
func TestWaitForSession_perToken(t *testing.T) {
	es := NewGrpcSender()
	go func() {
		time.Sleep(20 * time.Millisecond)
		es.SetSessionServer("other", &fakeStream{})
	}()
	require.False(t, es.WaitForSession(context.Background(), "tok", 150*time.Millisecond),
		"attaching another token must not satisfy this one")
}
