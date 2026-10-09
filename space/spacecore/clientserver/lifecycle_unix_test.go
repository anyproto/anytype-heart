//go:build !windows

package clientserver

import (
	"context"
	"net"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

func TestRunLifecycleWiring(t *testing.T) {
	t.Run("with the lifecycle on, yamux gets one wrapper that survives background and foreground on the same port", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.lifecycle = true
		fx.SetPauseOnBackground(true)
		fx.storage = failingSetStore{}

		// when
		require.NoError(t, fx.Run(context.Background()))
		t.Cleanup(func() { _ = fx.Close(context.Background()) })

		// then
		require.Len(t, fx.yamux.listeners, 1)
		lan, ok := fx.yamux.listeners[0].(*lanListener)
		require.True(t, ok, "yamux must get the wrapper, not the raw socket")
		done := serveMarker(lan)
		t.Cleanup(func() { _ = lan.Close(); <-done })
		addr := "127.0.0.1:" + strconv.Itoa(fx.Port())
		require.NoError(t, readMarker(addr))

		fx.StateChange(int(domain.CompStateAppWentBackground))
		assert.ErrorIs(t, readMarker(addr), syscall.ECONNREFUSED)

		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Eventually(t, func() bool { return readMarker(addr) == nil }, 2*time.Second, 20*time.Millisecond)
		require.Len(t, fx.yamux.listeners, 1, "the rebind must not register another listener")
	})

	t.Run("with the lifecycle off, yamux gets the raw socket and state changes do nothing", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.lifecycle = false
		fx.storage = failingSetStore{}

		// when
		require.NoError(t, fx.Run(context.Background()))
		fx.StateChange(int(domain.CompStateAppWentBackground))

		// then
		require.Len(t, fx.yamux.listeners, 1)
		_, isWrapper := fx.yamux.listeners[0].(*lanListener)
		assert.False(t, isWrapper)
		assert.Nil(t, fx.lc.cancel, "no worker without the lifecycle")
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(fx.Port()), time.Second)
		require.NoError(t, err)
		_ = conn.Close()
	})
}

// syscallFakeListener is a fake socket the production probe can inspect:
// probeSocket reaches getsockoptInt through it, which the test scripts.
type syscallFakeListener struct {
	*fakeListener
}

func (syscallFakeListener) SyscallConn() (syscall.RawConn, error) { return fakeRawConn{}, nil }

type fakeRawConn struct{}

func (fakeRawConn) Control(f func(fd uintptr)) error  { f(3); return nil }
func (fakeRawConn) Read(func(fd uintptr) bool) error  { return nil }
func (fakeRawConn) Write(func(fd uintptr) bool) error { return nil }

func TestWatchdogProductionProbe(t *testing.T) {
	t.Run("the default watchdog finds a socket error through the real probe and rebinds", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given: production probe and interval; the socket reports EBADF,
			// as iOS sets on a defuncted socket
			inner := syscallFakeListener{newFakeListener()}
			fx := &lifecycleFixture{
				clientServer: &clientServer{lifecycle: true, port: testLanPort},
				first:        inner.fakeListener,
			}
			fx.lan = newLanListener(inner, fx.lc.kickChan())
			fx.lc.listen = fx.listen
			var soErr atomic.Int64
			getsockoptInt = func(int, int, int) (int, error) { return int(soErr.Swap(0)), nil }
			t.Cleanup(func() { getsockoptInt = defaultGetsockoptInt })
			fx.startLifecycle()
			t.Cleanup(fx.stopLifecycle)
			synctest.Wait()

			// when
			soErr.Store(int64(syscall.EBADF))
			time.Sleep(20 * time.Second) // the specified interval, not the constant
			synctest.Wait()
			require.Equal(t, []int{testLanPort}, fx.ports(), "the watchdog must probe within 20 s")

			// then
			assert.Equal(t, []int{testLanPort}, fx.ports())
			assert.Same(t, fx.lastBound(), fx.current())
		})
	})
}
