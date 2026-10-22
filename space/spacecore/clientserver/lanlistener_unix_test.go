//go:build !windows

package clientserver

import (
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// serveMarker accepts on l and writes a marker to every connection, until l
// is closed; done is closed when the accept loop exited.
func serveMarker(l net.Listener) (done chan struct{}) {
	done = make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			_, _ = conn.Write([]byte("ok"))
			_ = conn.Close()
		}
	}()
	return done
}

// readMarker dials addr and reads the marker, proving the wrapper's Accept
// served the connection (a TCP handshake alone only proves the backlog).
func readMarker(addr string) error {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 2)
	if _, err = io.ReadFull(conn, buf); err != nil {
		return err
	}
	if string(buf) != "ok" {
		return errors.New("unexpected marker")
	}
	return nil
}

// Real sockets do not block durably inside synctest, so these run outside it.
func TestLanListenerRealSockets(t *testing.T) {
	t.Run("pause refuses new connections and rebind serves the same port", func(t *testing.T) {
		// given
		first, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		addr := first.Addr().String()
		l := newLanListener(first, make(chan struct{}, 1))
		done := serveMarker(l)
		t.Cleanup(func() {
			_ = l.Close()
			<-done
		})
		require.NoError(t, readMarker(addr))

		// when
		l.pause()

		// then
		err = readMarker(addr)
		assert.ErrorIs(t, err, syscall.ECONNREFUSED, "a paused listener must refuse, not hang")

		// when
		next, err := net.Listen("tcp", addr)
		require.NoError(t, err, "the same port must be free right after pause")
		require.NoError(t, l.install(next))

		// then
		assert.NoError(t, readMarker(addr))
	})
}

func TestProbeSocket(t *testing.T) {
	t.Run("a healthy socket passes", func(t *testing.T) {
		// given
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = lis.Close() })

		// when
		err = probeSocket(lis)

		// then
		assert.NoError(t, err)
	})

	t.Run("a pending socket error fails it", func(t *testing.T) {
		// given: what iOS sets on a defuncted socket
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = lis.Close() })
		getsockoptInt = func(int, int, int) (int, error) { return int(syscall.EBADF), nil }
		t.Cleanup(func() { getsockoptInt = defaultGetsockoptInt })

		// when
		err = probeSocket(lis)

		// then
		assert.ErrorIs(t, err, syscall.EBADF)
	})

	t.Run("a failing getsockopt fails it", func(t *testing.T) {
		// given
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		t.Cleanup(func() { _ = lis.Close() })
		getsockoptInt = func(int, int, int) (int, error) { return 0, syscall.EINVAL }
		t.Cleanup(func() { getsockoptInt = defaultGetsockoptInt })

		// when
		err = probeSocket(lis)

		// then
		assert.ErrorIs(t, err, syscall.EINVAL)
	})

	t.Run("a closed socket fails it", func(t *testing.T) {
		// given
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		_ = lis.Close()

		// when
		err = probeSocket(lis)

		// then
		assert.Error(t, err)
	})

	t.Run("no socket fails the wrapper's probe", func(t *testing.T) {
		// given
		lis, err := net.Listen("tcp", "127.0.0.1:0")
		require.NoError(t, err)
		l := newLanListener(lis, make(chan struct{}, 1))
		l.pause()

		// when
		err = l.probe()

		// then
		assert.ErrorIs(t, err, errNoListener)
	})
}
