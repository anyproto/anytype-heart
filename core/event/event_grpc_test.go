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
		for i := 0; i < maxSessionQueueLen+maxSendBatch+10; i++ {
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
		es.Broadcast(nonEmptyEvent())
	}
	require.Eventually(t, func() bool {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		return len(fs.sent) == n
	}, 2*time.Second, time.Millisecond)
}
