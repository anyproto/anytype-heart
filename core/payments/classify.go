package payments

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	stdnet "net"
	"os"
	"syscall"

	"github.com/anyproto/any-sync/net"
	"github.com/anyproto/any-sync/net/secureservice/handshake"
	"github.com/anyproto/any-sync/net/transport"
	"github.com/hashicorp/yamux"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// errCallBudget is the cause of a budget-bounded payment call's context
var errCallBudget = errors.New("payment node call budget exceeded")

// errConnLost marks a fetch that ended because the connection went away
// while the caller was still waiting. drpc reports a manager terminated by
// io.EOF (yamux session shutdown, QUIC stream FIN) as a bare
// context.Canceled, which is indistinguishable from a cancellation without
// the caller's context, so the fetch site wraps it (normalizeFetchErr).
var errConnLost = errors.New("payment node connection lost")

type connLostError struct{ cause error }

func (e connLostError) Error() string   { return errConnLost.Error() + ": " + e.cause.Error() }
func (e connLostError) Unwrap() []error { return []error{errConnLost, e.cause} }

// transientTargets are errors meaning the payment node was briefly
// unreachable
var transientTargets = []error{
	errConnLost,
	context.DeadlineExceeded,
	errCallBudget,
	transport.ErrConnClosed,
	net.ErrUnableToConnect,
	ErrNoConnection,
	handshake.ErrDeadlineExceeded,
	io.EOF,
	io.ErrUnexpectedEOF,
	io.ErrClosedPipe,
	os.ErrDeadlineExceeded,
	stdnet.ErrClosed,
	// socket-level failures (dial, handshake I/O, established stream)
	syscall.ECONNRESET,
	syscall.ECONNABORTED,
	syscall.ECONNREFUSED,
	syscall.EPIPE,
	syscall.ETIMEDOUT,
	syscall.ENETUNREACH,
	syscall.EHOSTUNREACH,
	// yamux (the TCP fallback): any-sync normalizes only Open/Accept errors
	yamux.ErrSessionShutdown,
	yamux.ErrConnectionReset,
	yamux.ErrStreamClosed,
	yamux.ErrRemoteGoAway,
	yamux.ErrKeepAliveTimeout,
	yamux.ErrConnectionWriteTimeout,
}

// IsTransientError reports whether err means the payment node was briefly
// unreachable: a timeout or budget expiry, a closed, reset or lost
// connection (including a QUIC connection that died during an established
// RPC, which any-sync wraps into transport.ErrConnClosed), a failed dial or
// handshake I/O. Authentication, configuration and persistence errors are
// not transient, nor is a bare context.Canceled: without the caller's
// context it can't be told from a cancellation (see isTransientForCaller).
// Each part of a joined error is considered.
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	for _, target := range transientTargets {
		if errors.Is(err, target) {
			return true
		}
	}
	if drpc.ClosedError.Has(err) {
		return true
	}
	// a TLS handshake I/O failure (yamux dial) is HandshakeError{Err: ...}.
	// The pinned any-sync unwraps it, so errors.Is above already sees the
	// cause; this is a fallback for an any-sync without
	// HandshakeError.Unwrap.
	var he handshake.HandshakeError
	if errors.As(err, &he) && he.Err != nil && IsTransientError(he.Err) {
		return true
	}
	// a non-TLS reply to the ClientHello: a captive portal or an
	// intercepting middlebox, gone once the user gets through it
	var rhe tls.RecordHeaderError
	if errors.As(err, &rhe) {
		return true
	}
	var timeout interface{ Timeout() bool }
	return errors.As(err, &timeout) && timeout.Timeout()
}

// callerCanceled reports whether the caller itself gave up. A caller's own
// deadline is not: a node too slow for it is a timeout like the budget.
func callerCanceled(callerCtx context.Context) bool {
	return callerCtx != nil && errors.Is(callerCtx.Err(), context.Canceled)
}

// isTransientForCaller is IsTransientError with the caller's own
// cancellation checked first: the pool can replace caller cancellation with
// ErrUnableToConnect, and a waiter that gave up is not a node failure. With
// the caller still waiting, a context.Canceled came from below (a lost
// connection) and is transient.
func isTransientForCaller(callerCtx context.Context, err error) bool {
	if callerCanceled(callerCtx) {
		return false
	}
	return IsTransientError(err) || solelyCanceled(err)
}

// solelyCanceled: err is a context.Canceled and nothing else (every part of
// a joined error is one); a permanent error joined with it stays permanent
func solelyCanceled(err error) bool {
	if err == nil {
		return false
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		parts := joined.Unwrap()
		for _, part := range parts {
			if !solelyCanceled(part) {
				return false
			}
		}
		return len(parts) > 0
	}
	next := errors.Unwrap(err)
	if next == nil {
		return errors.Is(err, context.Canceled)
	}
	return solelyCanceled(next)
}

// normalizeFetchErr classifies a fetch error where the caller's context is
// known. callerGone: the caller itself canceled, which is not a resource
// outcome (its own deadline is a timeout outcome). A cancellation that came
// from below is wrapped into errConnLost so that later checks without the
// caller's context (refreshErrorCode, the middleware) agree it is transient.
func normalizeFetchErr(callerCtx context.Context, err error) (normalized error, callerGone bool) {
	if err == nil {
		return nil, false
	}
	if callerCanceled(callerCtx) {
		return err, true
	}
	if solelyCanceled(err) {
		return connLostError{cause: err}, false
	}
	return err, false
}

func refreshErrorCode(err error) model.MembershipV2RefreshError {
	switch {
	case err == nil:
		return model.MembershipV2_RefreshErrorNull
	case IsTransientError(err):
		return model.MembershipV2_RefreshErrorPaymentNode
	default:
		return model.MembershipV2_RefreshErrorUnknown
	}
}
