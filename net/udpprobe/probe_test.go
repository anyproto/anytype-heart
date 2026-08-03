package udpprobe

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deadLoopbackAddr grabs a loopback UDP port and releases it, so nothing
// listens on it at probe time.
func deadLoopbackAddr(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	addr := conn.LocalAddr().String()
	require.NoError(t, conn.Close())
	return addr
}

func liveLoopbackAddr(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn.LocalAddr().String()
}

func TestProbe(t *testing.T) {
	t.Run("dead loopback port is reported dead fast", func(t *testing.T) {
		// given
		addr := deadLoopbackAddr(t)

		// when
		start := time.Now()
		got := Probe(context.Background(), addr)

		// then
		assert.Equal(t, Dead, got)
		assert.Less(t, time.Since(start), defaultTimeout, "refusal must beat the probe deadline")
	})

	t.Run("silent listener is inconclusive", func(t *testing.T) {
		// given
		addr := liveLoopbackAddr(t)
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()

		// when
		got := Probe(ctx, addr)

		// then
		assert.Equal(t, Inconclusive, got)
	})

	t.Run("non-local address is inconclusive without network traffic", func(t *testing.T) {
		// when
		got := Probe(context.Background(), "8.8.8.8:4443")

		// then
		assert.Equal(t, Inconclusive, got)
	})

	t.Run("unparseable address is inconclusive", func(t *testing.T) {
		assert.Equal(t, Inconclusive, Probe(context.Background(), "not-an-addr"))
	})
}

func TestProbeable(t *testing.T) {
	assert.True(t, Probeable("127.0.0.1:1234"))
	assert.True(t, Probeable("[::1]:1234"))
	assert.True(t, Probeable("192.168.1.10:1234"))
	assert.True(t, Probeable("10.0.0.1:1234"))
	assert.True(t, Probeable("169.254.1.1:1234"))
	assert.False(t, Probeable("8.8.8.8:1234"))
	assert.False(t, Probeable("example.com:1234"))
	assert.False(t, Probeable("127.0.0.1"))
}

func TestAllDead(t *testing.T) {
	t.Run("all dead loopback ports", func(t *testing.T) {
		// given
		addrs := []string{deadLoopbackAddr(t), deadLoopbackAddr(t)}

		// when / then
		assert.True(t, AllDead(context.Background(), addrs))
	})

	t.Run("one live listener keeps the peer dialable", func(t *testing.T) {
		// given
		addrs := []string{deadLoopbackAddr(t), liveLoopbackAddr(t)}
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		defer cancel()

		// when / then
		assert.False(t, AllDead(ctx, addrs))
	})

	t.Run("empty list proves nothing", func(t *testing.T) {
		assert.False(t, AllDead(context.Background(), nil))
	})

	t.Run("non-probeable address disables the verdict", func(t *testing.T) {
		// given: a dead local port plus a public addr we cannot verify
		addrs := []string{deadLoopbackAddr(t), "8.8.8.8:" + strconv.Itoa(4443)}

		// when / then
		assert.False(t, AllDead(context.Background(), addrs))
	})
}
