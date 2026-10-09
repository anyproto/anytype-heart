//go:build !windows

package clientserver

import (
	"context"
	"net"
	"strconv"
	"syscall"
	"testing"
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
