package gateway

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

// assertServes proves the gateway answers on addr. An unhandled path is enough - it exercises the
// mux without touching the file mocks.
func assertServes(t *testing.T, addr string) {
	t.Helper()

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("http://" + addr + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGatewayAddr(t *testing.T) {
	t.Run("serves on the well-known port without remembering it", func(t *testing.T) {
		// given
		wellKnown := freeAddr(t)

		// when
		fx := newFixtureWithConfig(t, &fakeConfig{}, portOf(t, wellKnown))

		// then
		assert.Equal(t, wellKnown, fx.Addr())
		assertServes(t, fx.Addr())

		// and: remembering it would erase the fallback that worked when it was busy
		assert.Empty(t, fx.config.GatewayAddr())
	})

	t.Run("remembers a fallback port for the next run", func(t *testing.T) {
		// given
		taken := occupiedAddr(t)

		// when
		fx := newFixtureWithConfig(t, &fakeConfig{}, portOf(t, taken))

		// then
		assert.NotEmpty(t, fx.Addr())
		assert.Equal(t, fx.Addr(), fx.config.GatewayAddr())
		assertServes(t, fx.Addr())
	})

	t.Run("keeps the remembered fallback when the well-known port comes back", func(t *testing.T) {
		// given: a fallback learned on an earlier run, and a well-known port that is free again
		fallback := freeAddr(t)
		wellKnown := freeAddr(t)
		cfg := &fakeConfig{addr: fallback}

		// when
		fx := newFixtureWithConfig(t, cfg, portOf(t, wellKnown))

		// then: the well-known port wins, but the fallback survives for the next run that needs it
		require.Equal(t, wellKnown, fx.Addr())
		assert.Equal(t, fallback, cfg.GatewayAddr())
	})

	t.Run("falls back to the persisted port when the well-known one is taken", func(t *testing.T) {
		// given
		persisted := freeAddr(t)
		taken := occupiedAddr(t)

		// when
		fx := newFixtureWithConfig(t, &fakeConfig{addr: persisted}, portOf(t, taken))

		// then
		assert.Equal(t, persisted, fx.Addr())
	})

	t.Run("binds an OS-assigned port when nothing else is available", func(t *testing.T) {
		// given: the machine that could not start the app - every candidate unavailable
		persisted := occupiedAddr(t)
		taken := occupiedAddr(t)

		// when
		fx := newFixtureWithConfig(t, &fakeConfig{addr: persisted}, portOf(t, taken))

		// then
		assert.NotEqual(t, persisted, fx.Addr())
		assert.NotEqual(t, taken, fx.Addr())
		assertServes(t, fx.Addr())
		assert.Equal(t, fx.Addr(), fx.config.GatewayAddr())
	})

	t.Run("keeps its port when the well-known one frees up mid-session", func(t *testing.T) {
		// given: the well-known port is busy at start, so the gateway lands elsewhere
		wellKnown, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		fx := newFixtureWithConfig(t, &fakeConfig{}, portOf(t, wellKnown.Addr().String()))
		want := fx.Addr()
		require.NotEqual(t, wellKnown.Addr().String(), want)

		// when: the well-known port frees up and mobile cycles background/foreground
		require.NoError(t, wellKnown.Close())
		require.NoError(t, fx.stopServer())
		require.NoError(t, fx.startServer())

		// then: moving back would strand every URL clients have cached
		assert.Equal(t, want, fx.Addr())
		assertServes(t, want)
	})

	t.Run("rebinds after the listener stops working", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.mu.Lock()
		ln := fx.listener
		fx.mu.Unlock()

		// when: the socket dies under the running server
		require.NoError(t, ln.Close())

		// then: the dead listener is given up instead of serving nothing for the rest of the session
		require.Eventually(t, func() bool {
			fx.mu.Lock()
			defer fx.mu.Unlock()
			return fx.listener == nil && !fx.isServerStarted
		}, 5*time.Second, 10*time.Millisecond)

		// and: the next start brings it back
		require.NoError(t, fx.startServer())
		assertServes(t, fx.Addr())
	})

	t.Run("does not keep serving on connections after the listener stops working", func(t *testing.T) {
		// given: a client holding a keep-alive connection
		fx := newFixture(t)
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get("http://" + fx.Addr() + "/")
		require.NoError(t, err)
		_, err = io.Copy(io.Discard, resp.Body)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())

		// when: the socket dies under the running server
		fx.mu.Lock()
		ln := fx.listener
		fx.mu.Unlock()
		require.NoError(t, ln.Close())
		require.Eventually(t, func() bool {
			fx.mu.Lock()
			defer fx.mu.Unlock()
			return fx.listener == nil
		}, 5*time.Second, 10*time.Millisecond)

		// then: nothing else would ever shut that server down, so it would answer on the pooled
		// connection long after Close returned
		require.Eventually(t, func() bool {
			resp, err := client.Get("http://" + fx.Addr() + "/")
			if err != nil {
				return true
			}
			_ = resp.Body.Close()
			return false
		}, 5*time.Second, 50*time.Millisecond)
	})

	t.Run("reports a duplicate start instead of starting twice", func(t *testing.T) {
		// given: mobile reports the same foreground state more than once
		fx := newFixture(t)

		// when
		err := fx.startServer()

		// then: starting twice would leak both a listener and an http.Server
		require.ErrorIs(t, err, errGatewayAlreadyStarted)
		assertServes(t, fx.Addr())
	})

	t.Run("stops and restarts with the mobile state changes", func(t *testing.T) {
		// given
		fx := newFixture(t)
		addr := fx.Addr()
		wasMobile := isMobile
		isMobile = true
		t.Cleanup(func() { isMobile = wasMobile })

		// when
		fx.StateChange(int(domain.CompStateAppWentBackground))

		// then
		_, err := net.DialTimeout("tcp", addr, 5*time.Second)
		require.Error(t, err)

		// and
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, addr, fx.Addr())
		assertServes(t, addr)
	})

	t.Run("keeps the port across repeated stop and start cycles", func(t *testing.T) {
		// given: the cycle mobile goes through on every background/foreground switch. Clients cache
		// the gateway URL from AccountInfo and nothing can hand them a new one, so the port has to
		// come back. The gateway sits on a port that is not the default one, so that coming back to
		// it proves the current port is what gets asked for.
		want := freeAddr(t)
		fx := newFixtureWithConfig(t, &fakeConfig{addr: want}, portOf(t, occupiedAddr(t)))
		require.Equal(t, want, fx.Addr())

		for range 3 {
			// when
			require.NoError(t, fx.stopServer())
			require.NoError(t, fx.startServer())

			// then
			assert.Equal(t, want, fx.Addr())
			assertServes(t, want)
		}
	})

	t.Run("refuses connections while stopped", func(t *testing.T) {
		// given
		fx := newFixture(t)
		addr := fx.Addr()

		// when
		require.NoError(t, fx.stopServer())

		// then: a client fails fast instead of hanging on a socket nobody is accepting from
		_, err := net.DialTimeout("tcp", addr, 5*time.Second)
		assert.Error(t, err)
	})

	t.Run("takes another port when the old one is taken while stopped", func(t *testing.T) {
		// given
		fx := newFixture(t)
		addr := fx.Addr()
		require.NoError(t, fx.stopServer())

		squatter, err := net.Listen("tcp", addr)
		require.NoError(t, err)
		defer squatter.Close()

		// when
		require.NoError(t, fx.startServer())

		// then: a lost port is better than a gateway that serves nothing
		assert.NotEqual(t, addr, fx.Addr())
		assertServes(t, fx.Addr())
	})

	t.Run("releases the port on close", func(t *testing.T) {
		// given
		fx := newFixture(t)
		addr := fx.Addr()

		// when
		require.NoError(t, fx.Close(context.Background()))

		// then
		ln, err := net.Listen("tcp", addr)
		require.NoError(t, err)
		require.NoError(t, ln.Close())
	})

	t.Run("refuses to start again after close", func(t *testing.T) {
		// given: mobile can still report a foreground switch while the app is shutting down
		fx := newFixture(t)
		require.NoError(t, fx.Close(context.Background()))

		// when
		err := fx.startServer()

		// then
		require.ErrorIs(t, err, errGatewayClosed)
		fx.mu.Lock()
		defer fx.mu.Unlock()
		assert.Nil(t, fx.listener)
	})

	t.Run("honours ANYTYPE_GATEWAY_ADDR without persisting it", func(t *testing.T) {
		// given
		want := freeAddr(t)
		t.Setenv("ANYTYPE_GATEWAY_ADDR", want)

		// when
		fx := newFixtureWithConfig(t, &fakeConfig{}, portOf(t, freeAddr(t)))

		// then
		assert.Equal(t, want, fx.Addr())
		assertServes(t, want)

		// and: persisting it would make later runs without the variable keep asking for that port
		assert.Empty(t, fx.config.GatewayAddr())
	})
}
