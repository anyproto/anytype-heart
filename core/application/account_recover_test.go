package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

// streamSender stands in for the gRPC sender: events only reach a client once
// its ListenSessionEvents stream has attached, and anything broadcast while no
// session is listening is dropped — that drop is what "no servers to broadcast
// event" means in the log. Every token-keyed answer is token-accurate, so a
// regression that waited on the wrong session would fail these tests.
type streamSender struct {
	mu        sync.Mutex
	attached  map[string]bool
	waiters   map[string][]chan struct{}
	received  []*pb.Event
	dropped   int
	waitCalls int
}

func newStreamSender() *streamSender {
	return &streamSender{attached: map[string]bool{}, waiters: map[string][]chan struct{}{}}
}

// attach registers one client's event stream, releasing only the waiters parked
// on that same token.
func (f *streamSender) attach(token string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attached[token] = true
	for _, ch := range f.waiters[token] {
		close(ch)
	}
	delete(f.waiters, token)
}

func (f *streamSender) WaitForSession(ctx context.Context, token string, timeout time.Duration) bool {
	f.mu.Lock()
	f.waitCalls++
	if f.attached[token] {
		f.mu.Unlock()
		return true
	}
	ch := make(chan struct{})
	f.waiters[token] = append(f.waiters[token], ch)
	f.mu.Unlock()

	select {
	case <-ch:
		return true
	case <-ctx.Done():
		return false
	case <-time.After(timeout):
		return false
	}
}

func (f *streamSender) Broadcast(e *pb.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.attached) == 0 {
		f.dropped++
		return
	}
	f.received = append(f.received, e)
}

func (f *streamSender) events() []*pb.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.received
}

func (f *streamSender) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.waitCalls
}

func (f *streamSender) Init(*app.App) error { return nil }
func (f *streamSender) Name() string        { return event.CName }
func (f *streamSender) IsActive(token string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.attached[token]
}
func (f *streamSender) SendToSession(string, *pb.Event)             {}
func (f *streamSender) BroadcastToOtherSessions(string, *pb.Event)  {}
func (f *streamSender) BroadcastExceptSessions(*pb.Event, []string) {}

var (
	_ event.Sender        = (*streamSender)(nil)
	_ event.SessionWaiter = (*streamSender)(nil)
)

func newRecoverFixture(t *testing.T) (*Service, *streamSender, string) {
	t.Helper()
	s := New()
	mnemonic, err := core.WalletGenerateMnemonic(wordCount)
	require.NoError(t, err)
	account, err := core.WalletAccountAt(mnemonic, 0)
	require.NoError(t, err)
	s.derivedKeys = &account
	sender := newStreamSender()
	s.eventSender = sender
	return s, sender, account.Identity.GetPublic().Account()
}

func accountShow(t *testing.T, ev *pb.Event) *pb.EventAccountShow {
	t.Helper()
	require.Len(t, ev.Messages, 1)
	value, ok := ev.Messages[0].Value.(*pb.EventMessageValueOfAccountShow)
	require.True(t, ok)
	return value.AccountShow
}

// blocked reports whether the call is still in flight after a grace period.
func blocked(t *testing.T, done <-chan error) bool {
	t.Helper()
	select {
	case <-done:
		return false
	case <-time.After(100 * time.Millisecond):
		return true
	}
}

func TestService_AccountRecover(t *testing.T) {
	t.Run("waits for the caller's event stream before broadcasting", func(t *testing.T) {
		// given a client that issued AccountRecover before its stream attached —
		// the two are independent requests and this is the order that loses
		s, sender, accountId := newRecoverFixture(t)

		// when
		done := make(chan error, 1)
		go func() { done <- s.AccountRecover(context.Background(), "tok") }()
		require.True(t, blocked(t, done), "broadcast into an empty session map: the event is lost")
		sender.attach("tok")

		// then the client gets the one event its login screen waits for
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("AccountRecover did not return after the stream attached")
		}
		require.Len(t, sender.events(), 1)
		assert.Equal(t, accountId, accountShow(t, sender.events()[0]).Account.Id)
		assert.Zero(t, sender.dropped)
	})

	t.Run("another session's stream does not release the wait", func(t *testing.T) {
		// given a second window already listening on its own session
		s, sender, _ := newRecoverFixture(t)

		// when
		done := make(chan error, 1)
		go func() { done <- s.AccountRecover(context.Background(), "tok") }()
		sender.attach("other")

		// then we are still waiting for the session that actually asked
		require.True(t, blocked(t, done), "waited on the wrong session")
		sender.attach("tok")
		require.NoError(t, <-done)
	})

	t.Run("answers about the account it was asked for, not one recovered mid-wait", func(t *testing.T) {
		// given a request parked on its stream
		s, sender, accountId := newRecoverFixture(t)
		done := make(chan error, 1)
		go func() { done <- s.AccountRecover(context.Background(), "tok") }()
		require.True(t, blocked(t, done))

		// when a second wallet recovery lands during the wait — WalletRecover needs
		// no session token, so any local process can do this
		other, err := core.WalletGenerateMnemonic(wordCount)
		require.NoError(t, err)
		require.NoError(t, s.WalletRecover(&pb.RpcWalletRecoverRequest{RootPath: t.TempDir(), Mnemonic: other}))
		sender.attach("tok")

		// then the login screen is offered the account this call was made for
		require.NoError(t, <-done)
		require.Len(t, sender.events(), 1)
		assert.Equal(t, accountId, accountShow(t, sender.events()[0]).Account.Id)
	})

	t.Run("broadcasts immediately when the stream is already attached", func(t *testing.T) {
		// given
		s, sender, accountId := newRecoverFixture(t)
		sender.attach("tok")

		// when
		err := s.AccountRecover(context.Background(), "tok")

		// then
		require.NoError(t, err)
		require.Len(t, sender.events(), 1)
		assert.Equal(t, accountId, accountShow(t, sender.events()[0]).Account.Id)
	})

	t.Run("no session token skips the wait", func(t *testing.T) {
		// given a caller whose transport carries no token (the mobile library)
		s, sender, accountId := newRecoverFixture(t)
		sender.attach("other")

		// when
		err := s.AccountRecover(context.Background(), "")

		// then it must not park on a session it cannot name
		require.NoError(t, err)
		assert.Zero(t, sender.calls())
		require.Len(t, sender.events(), 1)
		assert.Equal(t, accountId, accountShow(t, sender.events()[0]).Account.Id)
	})

	t.Run("gives up and broadcasts anyway when the stream never attaches", func(t *testing.T) {
		// given a caller that went away before its stream ever arrived
		s, sender, _ := newRecoverFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// when
		err := s.AccountRecover(ctx, "tok")

		// then the broadcast still happens — never worse than the pre-fix behaviour
		require.NoError(t, err)
		assert.Equal(t, 1, sender.calls())
		assert.Empty(t, sender.events())
		assert.Equal(t, 1, sender.dropped)
	})

	t.Run("no wallet", func(t *testing.T) {
		// given
		s := New()
		s.eventSender = newStreamSender()

		// when
		err := s.AccountRecover(context.Background(), "tok")

		// then
		assert.ErrorIs(t, err, ErrWalletNotInitialized)
	})
}
