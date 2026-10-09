package clientserver

import (
	"context"
	"errors"
	"net"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

const testLanPort = 5555

// lifecycleFixture is a clientServer with the iOS lifecycle on, a fake socket
// and a scripted listen.
type lifecycleFixture struct {
	*clientServer
	first *fakeListener

	mu sync.Mutex
	// listenPorts records every rebind attempt's port
	listenPorts []int
	// listenErrs are returned by the next listen calls, in order
	listenErrs []error
	// listenGate, when set, blocks listen until closed
	listenGate chan struct{}
	// listenHook, when set, runs inside listen before it returns
	listenHook func()
	bound      []*fakeListener
	// listenTimes records when each listen call happened
	listenTimes []time.Time
}

func newLifecycleFixture(t *testing.T) *lifecycleFixture {
	fx := &lifecycleFixture{
		clientServer: &clientServer{lifecycle: true, port: testLanPort},
		first:        newFakeListener(),
	}
	// most cases test the native-discovery policy; the default (no pause)
	// has its own cases
	fx.lc.pauseOnBackground = true
	fx.lan = newLanListener(fx.first, fx.lc.kickChan())
	fx.lc.listen = fx.listen
	fx.lc.watchdog = time.Hour
	return fx
}

// listen fails like a real bind while any earlier socket still holds the port.
func (fx *lifecycleFixture) listen(port int) (net.Listener, error) {
	fx.mu.Lock()
	fx.listenPorts = append(fx.listenPorts, port)
	fx.listenTimes = append(fx.listenTimes, time.Now())
	held := !fx.first.isClosed()
	for _, b := range fx.bound {
		held = held || !b.isClosed()
	}
	gate, hook := fx.listenGate, fx.listenHook
	if held {
		fx.mu.Unlock()
		return nil, syscall.EADDRINUSE
	}
	var err error
	if len(fx.listenErrs) > 0 {
		err, fx.listenErrs = fx.listenErrs[0], fx.listenErrs[1:]
	}
	fx.mu.Unlock()
	if gate != nil {
		<-gate
	}
	if hook != nil {
		hook()
	}
	if err != nil {
		return nil, err
	}
	lis := newFakeListener()
	fx.mu.Lock()
	fx.bound = append(fx.bound, lis)
	fx.mu.Unlock()
	return lis, nil
}

func (fx *lifecycleFixture) times() []time.Time {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	return append([]time.Time(nil), fx.listenTimes...)
}

func (fx *lifecycleFixture) intervals() []time.Duration {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	var res []time.Duration
	for i := 1; i < len(fx.listenTimes); i++ {
		res = append(res, fx.listenTimes[i].Sub(fx.listenTimes[i-1]))
	}
	return res
}

func (fx *lifecycleFixture) ports() []int {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	return append([]int(nil), fx.listenPorts...)
}

func (fx *lifecycleFixture) lastBound() *fakeListener {
	fx.mu.Lock()
	defer fx.mu.Unlock()
	if len(fx.bound) == 0 {
		return nil
	}
	return fx.bound[len(fx.bound)-1]
}

func (fx *lifecycleFixture) current() net.Listener {
	fx.lan.mu.Lock()
	defer fx.lan.mu.Unlock()
	return fx.lan.inner
}

func (fx *lifecycleFixture) start(t *testing.T) {
	fx.startLifecycle()
	t.Cleanup(fx.stopLifecycle)
	synctest.Wait()
}

func TestListenerLifecycle(t *testing.T) {
	t.Run("Background closes the socket before StateChange returns", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newLifecycleFixture(t)
			fx.start(t)

			// when
			fx.StateChange(int(domain.CompStateAppWentBackground))

			// then: no waiting for the worker
			assert.True(t, fx.first.isClosed())
			assert.Nil(t, fx.current())
		})
	})

	t.Run("Foreground rebinds the same port", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newLifecycleFixture(t)
			fx.start(t)
			fx.StateChange(int(domain.CompStateAppWentBackground))

			// when
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()

			// then
			assert.Equal(t, []int{testLanPort}, fx.ports())
			require.NotNil(t, fx.lastBound())
			assert.Same(t, fx.lastBound(), fx.current())
		})
	})

	t.Run("a duplicate Foreground with a healthy socket does nothing", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newLifecycleFixture(t)
			fx.start(t)

			// when
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()

			// then
			assert.Empty(t, fx.ports())
			assert.Same(t, fx.first, fx.current())
		})
	})

	t.Run("rebind retries with backoff and never picks another port", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newLifecycleFixture(t)
			fx.start(t)
			fx.listenErrs = []error{syscall.EADDRINUSE, syscall.EADDRINUSE}
			fx.StateChange(int(domain.CompStateAppWentBackground))
			synctest.Wait()

			// when
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()

			// then: attempts at 0, 50 and 150 ms, all on the saved port
			assert.Equal(t, []int{testLanPort}, fx.ports())
			time.Sleep(50 * time.Millisecond)
			synctest.Wait()
			assert.Equal(t, []int{testLanPort, testLanPort}, fx.ports())
			assert.Nil(t, fx.lastBound())
			time.Sleep(100 * time.Millisecond)
			synctest.Wait()
			assert.Equal(t, []int{testLanPort, testLanPort, testLanPort}, fx.ports())
			require.NotNil(t, fx.lastBound())
			assert.Same(t, fx.lastBound(), fx.current())
		})
	})

	t.Run("retries back off up to the cap, still on the saved port", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given: nine failures, past the end of the backoff table
			fx := newLifecycleFixture(t)
			fx.start(t)
			for range 9 {
				fx.listenErrs = append(fx.listenErrs, syscall.EADDRINUSE)
			}
			fx.StateChange(int(domain.CompStateAppWentBackground))
			synctest.Wait()
			start := time.Now()

			// when
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()
			// 50+100+200+400 ms + 1+2+5+5+5 s, then the tenth attempt binds
			time.Sleep(18750 * time.Millisecond)
			synctest.Wait()

			// then
			assert.Len(t, fx.ports(), 10)
			for _, port := range fx.ports() {
				assert.Equal(t, testLanPort, port)
			}
			require.NotNil(t, fx.lastBound())
			assert.Same(t, fx.lastBound(), fx.current())
			ms := time.Millisecond
			want := []time.Duration{50 * ms, 100 * ms, 200 * ms, 400 * ms, time.Second, 2 * time.Second, 5 * time.Second, 5 * time.Second, 5 * time.Second}
			assert.Equal(t, want, fx.intervals(), "each retry waits exactly its backoff step")
			assert.Equal(t, 18750*time.Millisecond, time.Since(start))
		})
	})

	t.Run("a Background during a backoff wait stops the retries", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given: the first bind fails, the worker waits 50 ms
			fx := newLifecycleFixture(t)
			fx.start(t)
			fx.listenErrs = []error{syscall.EADDRINUSE}
			fx.StateChange(int(domain.CompStateAppWentBackground))
			synctest.Wait() // let the Background settle, so one kick drives the Foreground
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()

			// when
			fx.StateChange(int(domain.CompStateAppWentBackground))
			time.Sleep(10 * time.Second)
			synctest.Wait()

			// then
			assert.Len(t, fx.ports(), 1)
			assert.Nil(t, fx.current())
		})
	})

	t.Run("a Foreground during a long backoff wait retries at once", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given: six failures, so the worker is in a 2 s wait
			fx := newLifecycleFixture(t)
			fx.start(t)
			for range 6 {
				fx.listenErrs = append(fx.listenErrs, syscall.EADDRINUSE)
			}
			fx.StateChange(int(domain.CompStateAppWentBackground))
			synctest.Wait() // let the Background settle, so one kick drives the Foreground
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()
			time.Sleep(1750 * time.Millisecond) // 50+100+200+400+1000 ms: five failures
			synctest.Wait()
			time.Sleep(time.Millisecond)
			synctest.Wait()
			require.Len(t, fx.ports(), 6, "precondition: the sixth attempt failed and a 2 s wait began")

			// when: Background and Foreground again; the first attempt after
			// them still fails
			fx.listenErrs = []error{syscall.EADDRINUSE}
			kicked := time.Now()
			fx.StateChange(int(domain.CompStateAppWentBackground))
			synctest.Wait() // the Background also wakes the worker out of its wait
			require.Len(t, fx.ports(), 6, "a Background makes no bind attempt")
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()

			// then: no waiting for the old 2 s timer, and the backoff starts
			// over: the next retry comes 50 ms later
			require.Len(t, fx.ports(), 7)
			assert.Equal(t, kicked, fx.times()[6], "the kick retries at once")
			time.Sleep(50 * time.Millisecond)
			synctest.Wait()
			require.Len(t, fx.ports(), 8)
			assert.Equal(t, 50*time.Millisecond, fx.intervals()[6], "the backoff restarted at 50 ms")
			assert.Same(t, fx.lastBound(), fx.current())
		})
	})

	t.Run("a duplicate Foreground during a long backoff retries at once and restarts the backoff", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given: six failures, so the worker is in a 2 s wait
			fx := newLifecycleFixture(t)
			fx.start(t)
			for range 6 {
				fx.listenErrs = append(fx.listenErrs, syscall.EADDRINUSE)
			}
			fx.StateChange(int(domain.CompStateAppWentBackground))
			synctest.Wait()
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()
			time.Sleep(1751 * time.Millisecond)
			synctest.Wait()
			require.Len(t, fx.ports(), 6, "precondition: the sixth attempt failed and a 2 s wait began")

			// when: another Foreground, no Background; the next attempt fails too
			fx.listenErrs = []error{syscall.EADDRINUSE}
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()

			// then: retried at once, and the following retry 50 ms later
			require.Len(t, fx.ports(), 7)
			time.Sleep(50 * time.Millisecond)
			synctest.Wait()
			require.Len(t, fx.ports(), 8, "the backoff restarts at 50 ms after a kick")
			assert.Same(t, fx.lastBound(), fx.current())
		})
	})

	t.Run("Close during a backoff wait stops the worker and closes the listener", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newLifecycleFixture(t)
			fx.startLifecycle()
			synctest.Wait()
			fx.listenErrs = []error{syscall.EADDRINUSE}
			fx.StateChange(int(domain.CompStateAppWentBackground))
			synctest.Wait() // let the Background settle, so one kick drives the Foreground
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()

			// when
			require.NoError(t, fx.Close(context.Background()))

			// then
			assert.Len(t, fx.ports(), 1)
			_, err := fx.lan.Accept()
			assert.ErrorIs(t, err, net.ErrClosed)
		})
	})

	t.Run("a failed watchdog probe rebinds", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newLifecycleFixture(t)
			fx.lc.watchdog = 20 * time.Second
			var probeMu sync.Mutex
			var probeErr error
			fx.lc.probe = func() error {
				probeMu.Lock()
				defer probeMu.Unlock()
				err := probeErr
				probeErr = nil // SO_ERROR is read-and-clear
				return err
			}
			fx.start(t)
			probeMu.Lock()
			probeErr = syscall.EBADF
			probeMu.Unlock()

			// when
			time.Sleep(20 * time.Second)
			synctest.Wait()

			// then
			assert.Equal(t, []int{testLanPort}, fx.ports())
			assert.Same(t, fx.lastBound(), fx.current())
		})
	})

	t.Run("a socket that died under Accept is rebound", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newLifecycleFixture(t)
			fx.start(t)
			go func() { _, _ = fx.lan.Accept() }()
			synctest.Wait()

			// when
			fx.first.errs <- &net.OpError{Op: "accept", Err: syscall.EBADF}
			synctest.Wait()

			// then
			assert.Equal(t, []int{testLanPort}, fx.ports())
			assert.Same(t, fx.lastBound(), fx.current())
			_ = fx.lan.Close()
		})
	})

	t.Run("a Background reported before the server started is applied on start", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given: a cold launch into the background
			fx := newLifecycleFixture(t)
			lan := fx.lan
			fx.lan = nil
			fx.StateChange(int(domain.CompStateAppWentBackground))
			fx.lan = lan

			// when
			fx.start(t)

			// then
			assert.True(t, fx.first.isClosed())
			assert.Nil(t, fx.current())
		})
	})

	t.Run("without the lifecycle StateChange changes nothing", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newLifecycleFixture(t)
			fx.lifecycle = false

			// when
			fx.StateChange(int(domain.CompStateAppWentBackground))

			// then
			assert.False(t, fx.first.isClosed())
		})
	})
}

// failingSetStore fails persisting while reads report no saved port.
type failingSetStore struct {
	keyvaluestore.Store[int]
}

func (failingSetStore) Get(context.Context, string) (int, error) { return 0, nil }
func (failingSetStore) Set(context.Context, string, int) error {
	return errors.New("disk full")
}

func TestStartServerPersistFailure(t *testing.T) {
	t.Run("a bound listener counts as started even if the port cannot be saved", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.storage = failingSetStore{}

		// when
		err := fx.Run(context.Background())

		// then
		require.NoError(t, err)
		assert.True(t, fx.ServerStarted())
		require.Len(t, fx.yamux.listeners, 1)
	})
}

// The bind holds lc.mu, which does not block durably inside synctest, so the
// contention cases run in real time.
func TestListenerLifecycleContention(t *testing.T) {
	t.Run("a Background while a socket is already bound waits for it and closes it", func(t *testing.T) {
		// given: listen has created the socket but not returned yet
		fx := newLifecycleFixture(t)
		bound := make(chan *fakeListener, 1)
		release := make(chan struct{})
		var releaseOnce sync.Once
		releaseBind := func() { releaseOnce.Do(func() { close(release) }) }
		// a failing assertion must not leave the worker stuck in listen,
		// or the cleanup that stops it would hang
		t.Cleanup(releaseBind)
		fx.lc.listen = func(int) (net.Listener, error) {
			lis := newFakeListener()
			bound <- lis
			<-release
			return lis, nil
		}
		fx.startLifecycle()
		t.Cleanup(func() {
			releaseBind()
			fx.stopLifecycle()
		})
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.StateChange(int(domain.CompStateAppWentForeground))
		live := <-bound

		// when
		backgroundDone := make(chan struct{})
		go func() {
			fx.StateChange(int(domain.CompStateAppWentBackground))
			close(backgroundDone)
		}()

		// then: Background cannot return while that socket is live
		select {
		case <-backgroundDone:
			t.Fatal("Background returned while a bound socket was not yet installed")
		case <-time.After(50 * time.Millisecond):
		}
		releaseBind()
		<-backgroundDone
		assert.True(t, live.isClosed(), "the socket bound during Background must be closed when it returns")
		assert.Nil(t, fx.current())
	})

	t.Run("Close during an in-flight bind leaves no socket open", func(t *testing.T) {
		// given
		fx := newLifecycleFixture(t)
		bound := make(chan *fakeListener, 1)
		release := make(chan struct{})
		var releaseOnce sync.Once
		releaseBind := func() { releaseOnce.Do(func() { close(release) }) }
		// a failing assertion must not leave the worker stuck in listen,
		// or the cleanup that stops it would hang
		t.Cleanup(releaseBind)
		fx.lc.listen = func(int) (net.Listener, error) {
			lis := newFakeListener()
			bound <- lis
			<-release
			return lis, nil
		}
		fx.startLifecycle()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.StateChange(int(domain.CompStateAppWentForeground))
		live := <-bound

		// when: Close has cancelled the worker before the bind returns
		cancelled := make(chan struct{})
		cancel := fx.lc.cancel
		fx.lc.cancel = func() {
			cancel()
			close(cancelled)
		}
		closeDone := make(chan error, 1)
		go func() { closeDone <- fx.Close(context.Background()) }()
		<-cancelled
		releaseBind()

		// then
		require.NoError(t, <-closeDone)
		assert.True(t, live.isClosed())
	})
}

func TestRebindAfterClose(t *testing.T) {
	t.Run("a closed listener is never rebound", func(t *testing.T) {
		// given
		fx := newLifecycleFixture(t)
		require.NoError(t, fx.lan.Close())

		// when
		bound, err := fx.rebindOnce()

		// then
		require.NoError(t, err)
		assert.False(t, bound)
		assert.Empty(t, fx.ports(), "no socket is opened for a closed listener")
	})

	t.Run("a socket bound while the listener closes is closed, not leaked", func(t *testing.T) {
		// given: Close lands between the bind and the install
		fx := newLifecycleFixture(t)
		fx.lan.pause()
		fx.listenHook = func() { _ = fx.lan.Close() }

		// when
		bound, err := fx.rebindOnce()

		// then
		require.NoError(t, err)
		assert.False(t, bound)
		require.NotNil(t, fx.lastBound())
		assert.True(t, fx.lastBound().isClosed())
	})

	t.Run("watchdog ticks after Close open no socket", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newLifecycleFixture(t)
			fx.lc.watchdog = 20 * time.Second
			fx.start(t)

			// when
			require.NoError(t, fx.lan.Close())
			time.Sleep(time.Minute)
			synctest.Wait()

			// then
			assert.Empty(t, fx.ports())
		})
	})
}

func TestListenerLifecycleWithoutPause(t *testing.T) {
	newDefaultFixture := func(t *testing.T) *lifecycleFixture {
		fx := newLifecycleFixture(t)
		fx.lc.pauseOnBackground = false
		return fx
	}

	t.Run("Background keeps the socket open", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newDefaultFixture(t)
			fx.start(t)

			// when
			fx.StateChange(int(domain.CompStateAppWentBackground))
			synctest.Wait()

			// then
			assert.False(t, fx.first.isClosed())
			assert.Same(t, fx.first, fx.current())
		})
	})

	t.Run("the Foreground after a Background rebinds the same port even if the probe passes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given: the old socket still holds the port, so a rebind that
			// skips closing it first fails
			fx := newDefaultFixture(t)
			fx.start(t)
			fx.StateChange(int(domain.CompStateAppWentBackground))

			// when
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()

			// then
			assert.True(t, fx.first.isClosed())
			assert.Equal(t, []int{testLanPort}, fx.ports())
			require.NotNil(t, fx.lastBound())
			assert.Same(t, fx.lastBound(), fx.current())
		})
	})

	t.Run("a Foreground without a Background before it does not rebind", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newDefaultFixture(t)
			fx.start(t)

			// when
			fx.StateChange(int(domain.CompStateAppWentForeground))
			synctest.Wait()

			// then
			assert.Empty(t, fx.ports())
		})
	})

	t.Run("turning pausing on applies from the next Background", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newDefaultFixture(t)
			fx.start(t)

			// when
			fx.SetPauseOnBackground(true)
			fx.StateChange(int(domain.CompStateAppWentBackground))

			// then
			assert.True(t, fx.first.isClosed())
			assert.Nil(t, fx.current())
		})
	})
}
