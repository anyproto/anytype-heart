package gateway

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
)

// The config component is what actually satisfies the gateway's addrStore at runtime, and it is
// resolved by interface, so a rename there would compile everywhere and panic on every app start.
var _ addrStore = (*config.Config)(nil)

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
	t.Run("persists the bound address for the next run", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// then
		assert.NotEmpty(t, fx.Addr())
		assert.Equal(t, fx.Addr(), fx.config.GatewayAddr())
		assertServes(t, fx.Addr())
	})

	t.Run("prefers the well-known port over the one the previous run persisted", func(t *testing.T) {
		// given: a persisted port that is free, so only the ranking decides
		persisted := freeAddr(t)
		wellKnown := freeAddr(t)

		// when
		fx := newFixtureWithConfig(t, &fakeConfig{addr: persisted}, portOf(t, wellKnown))

		// then: otherwise one busy start would strand us on an OS-assigned port for good
		assert.Equal(t, wellKnown, fx.Addr())
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
