package gateway

import (
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// an ambient override would send every fixture to the same address, and the second bind loses
	_ = os.Unsetenv("ANYTYPE_GATEWAY_ADDR")
	os.Exit(m.Run())
}

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

	t.Run("ignores a zero candidate", func(t *testing.T) {
		// given: what portFromAddr returns for an address a hand-edited config could hold
		want := freeAddr(t)

		// when
		ln, err := listenGateway(listenConfig{candidates: []int{0, portOf(t, want)}})

		// then: without the guard, 0 would bind an OS-assigned port and never try the real candidate
		require.NoError(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		assert.Equal(t, want, ln.Addr().String())
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

func TestPortFromAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want int
	}{
		{name: "an address we bound earlier", addr: "127.0.0.1:47800", want: 47800},
		{name: "nothing persisted yet", addr: "", want: 0},
		{name: "no port at all", addr: "127.0.0.1", want: 0},
		{name: "a named port", addr: "127.0.0.1:http", want: 0},
		{name: "the wildcard port", addr: "127.0.0.1:0", want: 0},
		{name: "a negative port", addr: "127.0.0.1:-1", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, portFromAddr(tt.addr))
		})
	}
}
