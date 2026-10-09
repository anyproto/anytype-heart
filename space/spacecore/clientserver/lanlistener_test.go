package clientserver

import (
	"net"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeListener hands out scripted connections and errors. With ignoreClose
// set, Close does not unblock a pending Accept, which lets a test deliver a
// connection on a socket that was already superseded.
type fakeListener struct {
	conns       chan net.Conn
	errs        chan error
	closed      chan struct{}
	closeOnce   sync.Once
	ignoreClose bool
}

func newFakeListener() *fakeListener {
	return &fakeListener{conns: make(chan net.Conn), errs: make(chan error), closed: make(chan struct{})}
}

func (f *fakeListener) Accept() (net.Conn, error) {
	closed := f.closed
	if f.ignoreClose {
		closed = nil
	}
	select {
	case c := <-f.conns:
		return c, nil
	case err := <-f.errs:
		return nil, err
	case <-closed:
		return nil, &net.OpError{Op: "accept", Err: net.ErrClosed}
	}
}

func (f *fakeListener) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func (f *fakeListener) Addr() net.Addr { return &net.TCPAddr{Port: 5555} }

func (f *fakeListener) isClosed() bool {
	select {
	case <-f.closed:
		return true
	default:
		return false
	}
}

type fakeConn struct {
	net.Conn
	mu     sync.Mutex
	closed bool
}

func (c *fakeConn) Close() error {
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	return nil
}

func (c *fakeConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

type temporaryError struct{}

func (temporaryError) Error() string   { return "temporary" }
func (temporaryError) Timeout() bool   { return false }
func (temporaryError) Temporary() bool { return true }

type acceptResult struct {
	conn net.Conn
	err  error
}

func acceptAsync(l *lanListener) chan acceptResult {
	res := make(chan acceptResult, 1)
	go func() {
		conn, err := l.Accept()
		res <- acceptResult{conn, err}
	}()
	return res
}

func TestLanListener(t *testing.T) {
	t.Run("passes connections through", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			inner := newFakeListener()
			l := newLanListener(inner, make(chan struct{}, 1))
			want := &fakeConn{}
			res := acceptAsync(l)

			// when
			inner.conns <- want

			// then
			got := <-res
			require.NoError(t, got.err)
			assert.Same(t, want, got.conn)
		})
	})

	t.Run("Accept survives a pause and serves the next socket", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			first := newFakeListener()
			l := newLanListener(first, make(chan struct{}, 1))
			res := acceptAsync(l)
			synctest.Wait()

			// when
			l.pause()
			synctest.Wait()
			second := newFakeListener()
			require.NoError(t, l.install(second))
			want := &fakeConn{}
			second.conns <- want

			// then
			got := <-res
			require.NoError(t, got.err)
			assert.Same(t, want, got.conn)
			assert.True(t, first.isClosed())
		})
	})

	t.Run("a connection accepted by a superseded socket is closed, not returned", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given: Accept is blocked on the first socket
			first := newFakeListener()
			first.ignoreClose = true
			l := newLanListener(first, make(chan struct{}, 1))
			res := acceptAsync(l)
			synctest.Wait()
			second := newFakeListener()
			require.NoError(t, l.install(second))

			// when: the old socket still delivers a connection
			stale := &fakeConn{}
			first.conns <- stale
			synctest.Wait()
			fresh := &fakeConn{}
			second.conns <- fresh

			// then
			got := <-res
			require.NoError(t, got.err)
			assert.Same(t, fresh, got.conn)
			assert.True(t, stale.isClosed(), "the stale connection must be closed")
		})
	})

	t.Run("Close ends a waiting Accept with net.ErrClosed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given: paused, so Accept waits for a socket
			l := newLanListener(newFakeListener(), make(chan struct{}, 1))
			l.pause()
			res := acceptAsync(l)
			synctest.Wait()

			// when
			require.NoError(t, l.Close())

			// then
			got := <-res
			assert.ErrorIs(t, got.err, net.ErrClosed)
		})
	})

	t.Run("Close ends an Accept blocked on a socket with net.ErrClosed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			inner := newFakeListener()
			l := newLanListener(inner, make(chan struct{}, 1))
			res := acceptAsync(l)
			synctest.Wait()

			// when
			require.NoError(t, l.Close())

			// then
			got := <-res
			assert.ErrorIs(t, got.err, net.ErrClosed)
			assert.True(t, inner.isClosed())
		})
	})

	t.Run("a dead socket requests a rebind and Accept keeps waiting", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			inner := newFakeListener()
			dead := make(chan struct{}, 1)
			l := newLanListener(inner, dead)
			res := acceptAsync(l)
			synctest.Wait()

			// when
			inner.errs <- &net.OpError{Op: "accept", Err: syscall.EBADF}
			synctest.Wait()

			// then
			select {
			case <-dead:
			default:
				t.Fatal("a dead socket must request a rebind")
			}
			assert.True(t, inner.isClosed())
			assert.Len(t, res, 0, "the error must not reach yamux")
			assert.ErrorIs(t, l.probe(), errNoListener)

			next := newFakeListener()
			require.NoError(t, l.install(next))
			want := &fakeConn{}
			next.conns <- want
			got := <-res
			require.NoError(t, got.err)
			assert.Same(t, want, got.conn)
		})
	})

	t.Run("temporary errors reach yamux unchanged", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			inner := newFakeListener()
			l := newLanListener(inner, make(chan struct{}, 1))
			res := acceptAsync(l)
			synctest.Wait()

			// when
			inner.errs <- temporaryError{}

			// then
			got := <-res
			assert.Equal(t, temporaryError{}, got.err)
			assert.False(t, inner.isClosed())
		})
	})

	t.Run("a connection accepted after a pause is closed and Accept keeps waiting", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given: Accept is blocked on a socket that ignores close
			first := newFakeListener()
			first.ignoreClose = true
			l := newLanListener(first, make(chan struct{}, 1))
			res := acceptAsync(l)
			synctest.Wait()

			// when: paused (Background), and the old socket still delivers
			l.pause()
			stale := &fakeConn{}
			first.conns <- stale
			synctest.Wait()

			// then
			assert.True(t, stale.isClosed(), "a connection from before the pause must not reach yamux")
			assert.Len(t, res, 0)
			require.NoError(t, l.Close())
			assert.ErrorIs(t, (<-res).err, net.ErrClosed)
		})
	})

	t.Run("an error from a superseded socket does not drop its replacement", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			first := newFakeListener()
			first.ignoreClose = true
			dead := make(chan struct{}, 1)
			l := newLanListener(first, dead)
			res := acceptAsync(l)
			synctest.Wait()
			second := newFakeListener()
			require.NoError(t, l.install(second))

			// when: the old socket fails after it was replaced
			first.errs <- &net.OpError{Op: "accept", Err: syscall.EBADF}
			synctest.Wait()

			// then
			assert.Len(t, dead, 0, "an obsolete error must not request a rebind")
			assert.False(t, second.isClosed())
			want := &fakeConn{}
			second.conns <- want
			got := <-res
			require.NoError(t, got.err)
			assert.Same(t, want, got.conn)
		})
	})

	t.Run("a connection accepted after Close is closed", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			first := newFakeListener()
			first.ignoreClose = true
			l := newLanListener(first, make(chan struct{}, 1))
			res := acceptAsync(l)
			synctest.Wait()

			// when
			require.NoError(t, l.Close())
			stale := &fakeConn{}
			first.conns <- stale

			// then
			assert.ErrorIs(t, (<-res).err, net.ErrClosed)
			assert.True(t, stale.isClosed())
		})
	})

	t.Run("install after Close is refused", func(t *testing.T) {
		// given
		l := newLanListener(newFakeListener(), make(chan struct{}, 1))
		require.NoError(t, l.Close())

		// when
		err := l.install(newFakeListener())

		// then
		assert.ErrorIs(t, err, net.ErrClosed)
	})
}
