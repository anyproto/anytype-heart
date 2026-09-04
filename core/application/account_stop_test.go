package application

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/recovery"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

// startFixture is a Service ready to select an account into a temp dir, with
// a sender whose Init — the first component a start reaches after config and
// wallet — blocks until the test releases it, standing in for a long app
// start under s.lock.
type startFixture struct {
	*Service
	accountId string
	rootPath  string
	entered   chan struct{} // closed when the start reaches the sender's Init
	enterOnce sync.Once
	release   chan error // what the sender's Init returns, once sent

	mu         sync.Mutex
	broadcasts []*pb.Event
}

func newStartFixture(t *testing.T) *startFixture {
	t.Helper()
	s := New()
	s.SetClientVersion("platform", "1")
	mnemonic, err := core.WalletGenerateMnemonic(wordCount)
	require.NoError(t, err)
	account, err := core.WalletAccountAt(mnemonic, 0)
	require.NoError(t, err)
	s.derivedKeys = &account
	fx := &startFixture{
		Service:   s,
		accountId: account.Identity.GetPublic().Account(),
		rootPath:  t.TempDir(),
		entered:   make(chan struct{}),
		release:   make(chan error),
	}
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Name().Return("service").Maybe()
	sender.EXPECT().Init(mock.Anything).RunAndReturn(func(*app.App) error {
		fx.enterOnce.Do(func() { close(fx.entered) })
		return <-fx.release
	}).Maybe()
	sender.EXPECT().Broadcast(mock.Anything).Run(func(ev *pb.Event) {
		fx.mu.Lock()
		defer fx.mu.Unlock()
		fx.broadcasts = append(fx.broadcasts, ev)
	}).Maybe()
	s.eventSender = sender
	return fx
}

func (fx *startFixture) selectRequest() *pb.RpcAccountSelectRequest {
	return &pb.RpcAccountSelectRequest{Id: fx.accountId, RootPath: fx.rootPath}
}

// inFlight is the start currently published for AccountStop to cancel, nil
// when there is none.
func (fx *startFixture) inFlight() *startRun {
	fx.startMu.Lock()
	defer fx.startMu.Unlock()
	return fx.starting
}

// stop runs AccountStop and fails the test if it does not return promptly:
// it must never wait for a start that holds s.lock.
func (fx *startFixture) stop(t *testing.T) error {
	t.Helper()
	stopped := make(chan error, 1)
	go func() { stopped <- fx.AccountStop(&pb.RpcAccountStopRequest{}) }()
	select {
	case err := <-stopped:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("AccountStop did not return: it waited for s.lock")
		return nil
	}
}

// verdicts counts the Failed phase changes broadcast so far: a cancelled
// start must publish none, whichever component the cancel struck first.
func (fx *startFixture) verdicts() int {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	n := 0
	for _, ev := range fx.broadcasts {
		for _, msg := range ev.Messages {
			v, ok := msg.Value.(*pb.EventMessageValueOfAccountRecoveryUpdate)
			if !ok {
				continue
			}
			if p, ok := v.AccountRecoveryUpdate.Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged); ok && p.PhaseChanged.Phase == pb.EventAccountRecovery_Failed {
				n++
			}
		}
	}
	return n
}

func (fx *startFixture) assertNeverBooted(t *testing.T) {
	t.Helper()
	select {
	case <-fx.entered:
		t.Fatal("the start reached app.Start")
	default:
	}
	assert.Nil(t, fx.GetApp())
	snapshot, err := fx.AccountRecoveryState()
	require.NoError(t, err)
	assert.Equal(t, recovery.IdleSnapshot(), snapshot, "no run was opened")
}

func TestService_AccountStop(t *testing.T) {
	t.Run("nothing running and nothing starting: the account is not running", func(t *testing.T) {
		// given
		fx := newStartFixture(t)

		// when
		err := fx.AccountStop(&pb.RpcAccountStopRequest{})

		// then
		require.ErrorIs(t, err, ErrApplicationIsNotRunning)
	})

	t.Run("stop before the start is cancellable is not dropped", func(t *testing.T) {
		// given: a select queued behind s.lock — the previous app's close, say
		fx := newStartFixture(t)
		fx.lock.Lock()
		selected := make(chan error, 1)
		go func() {
			_, err := fx.AccountSelect(context.Background(), fx.selectRequest())
			selected <- err
		}()
		// the run is published before the lock wait; this poll is monotonic:
		// were the publish behind the lock, it could only ever time out
		require.Eventually(t, func() bool { return fx.inFlight() != nil }, 5*time.Second, time.Millisecond)

		// when: the stop returns while the test still holds the lock
		err := fx.stop(t)
		fx.lock.Unlock()

		// then
		require.NoError(t, err, "cancelling a pending start is a successful stop")
		require.ErrorIs(t, <-selected, context.Canceled)
		fx.assertNeverBooted(t)
	})

	t.Run("stop during the start returns at once, and the start unwinds on its own", func(t *testing.T) {
		// given: a select holding s.lock inside app.Start
		fx := newStartFixture(t)
		errInit := errors.New("init gave up")
		selected := make(chan error, 1)
		go func() {
			_, err := fx.AccountSelect(context.Background(), fx.selectRequest())
			selected <- err
		}()
		<-fx.entered
		run := fx.inFlight()
		require.NotNil(t, run)

		// when: the stop returns while the start still holds s.lock
		err := fx.stop(t)

		// then
		require.NoError(t, err, "cancelling a pending start is a successful stop")
		require.ErrorIs(t, run.ctx.Err(), context.Canceled)
		fx.release <- errInit // the start unwinds, reporting its own error
		require.ErrorIs(t, <-selected, errInit)
		assert.Nil(t, fx.GetApp())
		snapshot, err := fx.AccountRecoveryState()
		require.NoError(t, err)
		assert.Equal(t, recovery.IdleSnapshot(), snapshot, "a cancelled run must not keep looking alive")
		assert.Equal(t, 0, fx.verdicts(), "a cancelled start publishes no verdict")
		err = fx.AccountStop(&pb.RpcAccountStopRequest{})
		require.ErrorIs(t, err, ErrApplicationIsNotRunning, "the cancel is reported once")
	})

	t.Run("a stop the start hears too late: the start closes the app it published", func(t *testing.T) {
		// given: a start that booted its app before the cancel reached it,
		// about to finish under s.lock
		fx := newStartFixture(t)
		_, end := fx.beginStart(context.Background())
		fx.lock.Lock()
		defer fx.lock.Unlock()
		fx.app = new(app.App)

		// when
		require.NoError(t, fx.stop(t))
		end()

		// then
		assert.Nil(t, fx.app)
		assert.Nil(t, fx.inFlight())
	})

	t.Run("a start that finished cleanly keeps its app and its context", func(t *testing.T) {
		// given
		fx := newStartFixture(t)
		ctx, end := fx.beginStart(context.Background())
		fx.lock.Lock()
		defer fx.lock.Unlock()
		fx.app = new(app.App)

		// when
		end()

		// then
		assert.NotNil(t, fx.app)
		assert.NoError(t, ctx.Err(), "the app's components run under this context")
		assert.Nil(t, fx.inFlight())
	})

	t.Run("a newer start supersedes the one in flight, and only the newest can be stopped", func(t *testing.T) {
		// given
		fx := newStartFixture(t)
		first, endFirst := fx.beginStart(context.Background())

		// when
		second, endSecond := fx.beginStart(context.Background())

		// then
		require.ErrorIs(t, first.Err(), context.Canceled)
		require.NoError(t, second.Err())

		// and the superseded start ending does not retract the newer one
		fx.lock.Lock()
		endFirst()
		fx.lock.Unlock()
		require.NotNil(t, fx.inFlight())
		require.NoError(t, fx.stop(t))
		require.ErrorIs(t, second.Err(), context.Canceled)
		fx.lock.Lock()
		endSecond()
		fx.lock.Unlock()
	})

	t.Run("process shutdown cancels a start in flight instead of waiting for it", func(t *testing.T) {
		// given
		fx := newStartFixture(t)
		ctx, end := fx.beginStart(context.Background())

		// when
		require.NoError(t, fx.Stop())

		// then
		require.ErrorIs(t, ctx.Err(), context.Canceled)
		fx.lock.Lock()
		end()
		fx.lock.Unlock()
	})
}

// TestService_AccountStopRemoveData pins that a removal request is never
// answered with a success it did not earn. Cancelling a start returns early
// without touching the lock, which is right for a plain stop but would tell a
// client its account had been erased while every byte was still on disk.
func TestService_AccountStopRemoveData(t *testing.T) {
	stopWith := func(t *testing.T, fx *startFixture, removeData bool) error {
		t.Helper()
		stopped := make(chan error, 1)
		go func() {
			stopped <- fx.AccountStop(&pb.RpcAccountStopRequest{RemoveData: removeData})
		}()
		select {
		case err := <-stopped:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("AccountStop did not return")
			return nil
		}
	}

	t.Run("a plain stop of a start in flight still succeeds without the lock", func(t *testing.T) {
		// given: a start in flight
		fx := newStartFixture(t)
		fx.startMu.Lock()
		fx.starting = &startRun{cancel: func() {}}
		fx.startMu.Unlock()

		// when
		err := stopWith(t, fx, false)

		// then
		assert.NoError(t, err)
		assert.Nil(t, fx.inFlight())
	})

	t.Run("a removal request on a cancelled start reports that it did not happen", func(t *testing.T) {
		// given: a start in flight and no app to remove data through
		fx := newStartFixture(t)
		fx.startMu.Lock()
		fx.starting = &startRun{cancel: func() {}}
		fx.startMu.Unlock()
		require.Nil(t, fx.app)

		// when
		err := stopWith(t, fx, true)

		// then: the start was still cancelled, but the caller is told the
		// truth rather than a success it can never verify
		assert.ErrorIs(t, err, ErrFailedToRemoveAccountData)
		assert.Nil(t, fx.inFlight())
	})
}
