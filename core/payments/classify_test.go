package payments

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	stdnet "net"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/anyproto/any-sync/net"
	"github.com/anyproto/any-sync/net/secureservice/handshake"
	"github.com/anyproto/any-sync/net/transport"
	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"storj.io/drpc"

	psp "github.com/anyproto/any-sync/paymentservice/paymentserviceproto"

	"github.com/anyproto/anytype-heart/core/payments/cache"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// connDeadLike mirrors any-sync's QUIC stream Read/Write normalization: the
// error matches transport.ErrConnClosed and still unwraps to the quic cause
type connDeadLike struct{ cause error }

func (e connDeadLike) Error() string   { return "conn closed: " + e.cause.Error() }
func (e connDeadLike) Unwrap() []error { return []error{transport.ErrConnClosed, e.cause} }

// quicIdleTimeout and quicStatelessReset stand in for the quic-go connection
// errors (quic-go is not a direct dependency of heart)
type quicIdleTimeout struct{}

func (quicIdleTimeout) Error() string   { return "timeout: no recent network activity" }
func (quicIdleTimeout) Timeout() bool   { return true }
func (quicIdleTimeout) Temporary() bool { return false }

type quicStatelessReset struct{}

func (quicStatelessReset) Error() string { return "received a stateless reset" }

// quicStreamReset is a stream-level reset: the connection itself is fine
type quicStreamReset struct{}

func (quicStreamReset) Error() string { return "stream 4 canceled by remote with error code 0" }

func TestIsTransientError(t *testing.T) {
	transient := map[string]error{
		"deadline":                     context.DeadlineExceeded,
		"budget":                       errCallBudget,
		"wrapped deadline":             fmt.Errorf("get status: %w", context.DeadlineExceeded),
		"conn closed at acquisition":   transport.ErrConnClosed,
		"QUIC read on dead conn":       connDeadLike{cause: quicIdleTimeout{}},
		"QUIC stateless reset":         connDeadLike{cause: quicStatelessReset{}},
		"raw QUIC idle timeout":        quicIdleTimeout{},
		"unable to connect":            net.ErrUnableToConnect,
		"wrapped unable to connect":    fmt.Errorf("pool: %w", net.ErrUnableToConnect),
		"no connection":                ErrNoConnection,
		"handshake deadline":           handshake.ErrDeadlineExceeded,
		"handshake EOF":                io.EOF,
		"handshake unexpected EOF":     io.ErrUnexpectedEOF,
		"i/o deadline":                 os.ErrDeadlineExceeded,
		"wrapped handshake I/O error":  fmt.Errorf("handshake: %w", io.ErrUnexpectedEOF),
		"conn lost (normalized)":       connLostError{cause: context.Canceled},
		"yamux session shutdown":       fmt.Errorf("manager closed: %w", yamux.ErrSessionShutdown),
		"yamux connection reset":       yamux.ErrConnectionReset,
		"drpc-wrapped yamux reset":     drpc.ProtocolError.Wrap(yamux.ErrConnectionReset), // errs.Class wrap, as "manager closed"
		"TLS handshake conn reset":     handshake.HandshakeError{Err: &stdnet.OpError{Op: "read", Err: os.NewSyscallError("read", syscall.ECONNRESET)}},
		"conn aborted":                 os.NewSyscallError("read", syscall.ECONNABORTED),
		"broken pipe":                  os.NewSyscallError("write", syscall.EPIPE),
		"timed out":                    os.NewSyscallError("connect", syscall.ETIMEDOUT),
		"net unreachable":              os.NewSyscallError("connect", syscall.ENETUNREACH),
		"host unreachable":             os.NewSyscallError("connect", syscall.EHOSTUNREACH),
		"yamux stream closed":          yamux.ErrStreamClosed,
		"yamux keepalive timeout":      yamux.ErrKeepAliveTimeout,
		"yamux write timeout":          yamux.ErrConnectionWriteTimeout,
		"captive portal reply":         handshake.HandshakeError{Err: tls.RecordHeaderError{Msg: "first record does not look like a TLS handshake"}},
		"dial refused":                 &stdnet.OpError{Op: "dial", Err: os.NewSyscallError("connect", syscall.ECONNREFUSED)},
		"yamux go away":                yamux.ErrRemoteGoAway,
		"TLS handshake EOF":            handshake.HandshakeError{Err: io.EOF},
		"TLS handshake unexpected EOF": handshake.HandshakeError{Err: io.ErrUnexpectedEOF},
		"TLS handshake i/o timeout":    fmt.Errorf("dial: %w", handshake.HandshakeError{Err: &stdnet.OpError{Op: "read", Err: os.ErrDeadlineExceeded}}),
		"drpc remote closed":           drpc.ClosedError.New("remote closed the stream"),
		"closed network connection":    stdnet.ErrClosed,
		"joined: canceled + transient": errors.Join(context.Canceled, net.ErrUnableToConnect),
	}
	for name, err := range transient {
		t.Run("transient: "+name, func(t *testing.T) {
			assert.True(t, IsTransientError(err))
			assert.Equal(t, model.MembershipV2_RefreshErrorPaymentNode, refreshErrorCode(err))
		})
	}

	notTransient := map[string]error{
		"nil":                            nil,
		"canceled":                       context.Canceled,
		"auth":                           psp.ErrInvalidSignature,
		"config":                         ErrV2NotEnabled,
		"persistence":                    cache.ErrCacheDbError,
		"generic":                        errors.New("boom"),
		"QUIC stream reset":              quicStreamReset{},
		"handshake proto":                handshake.ErrIncompatibleProto,
		"remote text mentioning a reset": errors.New("upstream: stream reset by stripe"),
		"TLS handshake bad credentials":  handshake.HandshakeError{Err: errors.New("remote peer declined the credentials")},
		"joined permanent + canceled":    errors.Join(psp.ErrInvalidSignature, context.Canceled),
	}
	for name, err := range notTransient {
		t.Run("not transient: "+name, func(t *testing.T) {
			assert.False(t, IsTransientError(err))
		})
	}

	t.Run("normalizeFetchErr", func(t *testing.T) {
		alive := context.Background()
		err, gone := normalizeFetchErr(alive, context.Canceled)
		assert.False(t, gone)
		assert.ErrorIs(t, err, errConnLost, "a cancellation from below is a lost connection")
		assert.ErrorIs(t, err, context.Canceled, "the cause is kept")
		assert.True(t, IsTransientError(err))

		err, gone = normalizeFetchErr(alive, psp.ErrInvalidSignature)
		assert.False(t, gone)
		assert.Equal(t, psp.ErrInvalidSignature, err)

		dead, cancel := context.WithCancel(context.Background())
		cancel()
		err, gone = normalizeFetchErr(dead, net.ErrUnableToConnect)
		assert.True(t, gone, "the caller gave up")
		assert.Equal(t, net.ErrUnableToConnect, err)

		joined := errors.Join(psp.ErrInvalidSignature, context.Canceled)
		err, gone = normalizeFetchErr(alive, joined)
		assert.False(t, gone)
		assert.Equal(t, joined, err, "only a solely-canceled error is a lost connection")
		assert.False(t, isTransientForCaller(alive, joined))

		short, cancelShort := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancelShort()
		<-short.Done()
		err, gone = normalizeFetchErr(short, context.DeadlineExceeded)
		assert.False(t, gone, "the caller's own deadline is a timeout outcome")
		assert.True(t, isTransientForCaller(short, err))

		err, gone = normalizeFetchErr(alive, nil)
		assert.NoError(t, err)
		assert.False(t, gone)
	})

	t.Run("caller cancellation is checked first", func(t *testing.T) {
		cctx, cancel := context.WithCancel(context.Background())
		assert.True(t, isTransientForCaller(cctx, net.ErrUnableToConnect))
		cancel()
		assert.False(t, isTransientForCaller(cctx, net.ErrUnableToConnect))
		assert.False(t, isTransientForCaller(cctx, context.DeadlineExceeded))
		assert.True(t, isTransientForCaller(context.Background(), context.Canceled), "canceled from below")
	})
}
