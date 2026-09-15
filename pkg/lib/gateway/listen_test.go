package gateway

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// occupiedAddr binds an address and holds it for the rest of the test, so that
// anything else trying to bind it fails.
func occupiedAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })

	return ln.Addr().String()
}

// freeAddr returns an address that is free at the moment of the call.
func freeAddr(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	return addr
}

func portOf(t *testing.T, addr string) int {
	t.Helper()

	_, port, err := net.SplitHostPort(addr)
	require.NoError(t, err)
	p, err := strconv.Atoi(port)
	require.NoError(t, err)

	return p
}

// assertBound proves the listener really owns the address it reports: the bug this
// replaces returned an address that had never been probed, let alone bound.
func assertBound(t *testing.T, ln net.Listener) {
	t.Helper()

	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	_ = conn.Close()
}

func TestListenGateway(t *testing.T) {
	t.Run("binds the explicitly requested address", func(t *testing.T) {
		// given
		want := freeAddr(t)

		// when
		ln, err := listenGateway(listenConfig{override: want})

		// then
		require.NoError(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		assert.Equal(t, want, ln.Addr().String())
		assertBound(t, ln)
	})

	t.Run("fails instead of substituting when the requested address is busy", func(t *testing.T) {
		// given
		busy := occupiedAddr(t)

		// when
		ln, err := listenGateway(listenConfig{override: busy})

		// then
		require.Error(t, err)
		assert.Nil(t, ln)
	})

	t.Run("binds the first free candidate", func(t *testing.T) {
		// given
		want := freeAddr(t)

		// when
		ln, err := listenGateway(listenConfig{candidates: []int{portOf(t, want), portOf(t, occupiedAddr(t))}})

		// then
		require.NoError(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		assert.Equal(t, want, ln.Addr().String())
	})

	t.Run("skips a busy candidate", func(t *testing.T) {
		// given
		busy := occupiedAddr(t)
		want := freeAddr(t)

		// when
		ln, err := listenGateway(listenConfig{candidates: []int{portOf(t, busy), portOf(t, want)}})

		// then
		require.NoError(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		assert.Equal(t, want, ln.Addr().String())
	})

	t.Run("binds an OS-assigned port when every candidate is busy", func(t *testing.T) {
		// given
		busyFirst := occupiedAddr(t)
		busySecond := occupiedAddr(t)

		// when
		ln, err := listenGateway(listenConfig{candidates: []int{portOf(t, busyFirst), portOf(t, busySecond)}})

		// then
		require.NoError(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		assert.NotEqual(t, busyFirst, ln.Addr().String())
		assert.NotEqual(t, busySecond, ln.Addr().String())
		assertBound(t, ln)
		host, _, err := net.SplitHostPort(ln.Addr().String())
		require.NoError(t, err)
		assert.Equal(t, gatewayHost, host)
	})

	t.Run("ignores an out-of-range candidate", func(t *testing.T) {
		// given
		want := freeAddr(t)

		// when
		ln, err := listenGateway(listenConfig{candidates: []int{99999, portOf(t, want)}})

		// then
		require.NoError(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		assert.Equal(t, want, ln.Addr().String())
	})
}
