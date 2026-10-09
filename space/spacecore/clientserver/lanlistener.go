package clientserver

import (
	"errors"
	"net"
	"sync"

	"go.uber.org/zap"
)

var errNoListener = errors.New("no listening socket")

// lanListener is the LAN TCP listener as yamux sees it: one net.Listener,
// registered once, whose underlying socket can be closed and replaced. iOS
// defuncts an app's sockets on suspension; a defunct listener answers RST to
// peers while Go's Accept parks forever (kqueue never fires for an empty
// accept queue), so the socket is closed on Background and re-created on
// Foreground (see docs/superpowers/specs/2026-10-09-ios-lan-socket-lifecycle-design.md).
//
// Accept never surfaces a pause, a rebind or a dead socket to yamux: it waits
// for the next socket instead. Only Close ends it, with net.ErrClosed.
type lanListener struct {
	addr net.Addr
	// dead receives a non-blocking signal when the current socket failed
	// with a non-temporary error, so the owner rebinds it
	dead chan<- struct{}

	mu   sync.Mutex
	cond *sync.Cond
	// inner is the current socket, nil while paused or dead
	inner net.Listener
	// gen changes whenever inner is replaced or dropped; an Accept result
	// from an older generation is obsolete
	gen    uint64
	closed bool
}

func newLanListener(inner net.Listener, dead chan<- struct{}) *lanListener {
	l := &lanListener{addr: inner.Addr(), dead: dead, inner: inner}
	l.cond = sync.NewCond(&l.mu)
	return l
}

func (l *lanListener) Accept() (net.Conn, error) {
	for {
		l.mu.Lock()
		for l.inner == nil && !l.closed {
			l.cond.Wait()
		}
		if l.closed {
			l.mu.Unlock()
			return nil, net.ErrClosed
		}
		inner, gen := l.inner, l.gen
		l.mu.Unlock()

		conn, err := inner.Accept()

		l.mu.Lock()
		closed, obsolete := l.closed, l.gen != gen
		if err == nil {
			l.mu.Unlock()
			if closed || obsolete {
				// accepted by a socket that was paused, replaced or closed
				// meanwhile: never hand it to yamux across that boundary
				_ = conn.Close()
				continue
			}
			return conn, nil
		}
		switch {
		case closed:
			l.mu.Unlock()
			return nil, net.ErrClosed
		case obsolete:
			// an intentional pause or rebind closed this socket
			l.mu.Unlock()
			continue
		case isTemporary(err):
			l.mu.Unlock()
			return nil, err
		}
		// the current socket died (e.g. EBADF from a defunct listener that
		// had a queued connection): drop it and ask for a rebind
		l.inner = nil
		l.gen++
		l.mu.Unlock()
		_ = inner.Close()
		log.Warn("lan listener socket died", zap.Error(err))
		select {
		case l.dead <- struct{}{}:
		default:
		}
	}
}

// pause closes the current socket. Go's Close wakes a goroutine parked in
// Accept on it, so this also releases an Accept stuck on a defunct fd.
func (l *lanListener) pause() {
	l.mu.Lock()
	inner := l.inner
	l.inner = nil
	l.gen++
	l.mu.Unlock()
	if inner != nil {
		_ = inner.Close()
	}
}

// install makes next the current socket. After Close it refuses with
// net.ErrClosed and the caller must close next itself.
func (l *lanListener) install(next net.Listener) error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return net.ErrClosed
	}
	old := l.inner
	l.inner = next
	l.gen++
	l.cond.Broadcast()
	l.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

// probe reports whether the current socket is healthy; see probeSocket.
func (l *lanListener) probe() error {
	l.mu.Lock()
	inner, closed := l.inner, l.closed
	l.mu.Unlock()
	if closed {
		return net.ErrClosed
	}
	if inner == nil {
		return errNoListener
	}
	return probeSocket(inner)
}

// isClosed reports whether Close ended the listener for good.
func (l *lanListener) isClosed() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closed
}

func (l *lanListener) Close() error {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return nil
	}
	l.closed = true
	inner := l.inner
	l.inner = nil
	l.gen++
	l.cond.Broadcast()
	l.mu.Unlock()
	if inner != nil {
		return inner.Close()
	}
	return nil
}

func (l *lanListener) Addr() net.Addr {
	return l.addr
}

// isTemporary mirrors the yamux transport's accept-loop check: a temporary
// error is returned and retried there after a pause.
func isTemporary(err error) bool {
	var netErr net.Error
	// nolint:staticcheck // Temporary is what the yamux accept loop retries on
	return errors.As(err, &netErr) && netErr.Temporary()
}
