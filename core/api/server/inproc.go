package server

// inproc.go — an http.RoundTripper that serves each request by calling the
// gin engine's ServeHTTP directly (the full-wrapper design,
// docs/superpowers/specs/2026-09-18-full-wrapper-in-heart-design.md §3.3).
// No listener, no socket, no port: the "HTTP" is Go structs passed between
// functions in one process, and every middleware between the transport and
// the service layer runs unchanged — origin policy, bearer authentication,
// the scope and grant gates, the idempotency store, the write rate limiter.
//
// The transport is engine-only. It never supplies a credential: the client
// that uses it sends whatever Authorization it was built with, so an inner
// /v2 call made on behalf of an /mcp caller carries that caller's own key
// and nothing wider. A test pins that a request without a bearer arrives at
// the engine without one.
//
// Responses are buffered whole and capped: the chat and search SSE streams
// are not served this way (the full tier excludes them), and a runaway body
// fails the round trip instead of growing without bound.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// InProcessBaseURL is the base URL a client over the in-process transport
// is built with. The host is an IP literal so the origin policy's Host
// check (localorigin.AllowHost) admits it; no port, because nothing listens.
const InProcessBaseURL = "http://127.0.0.1"

// inProcessRemoteAddr is the RemoteAddr stamped on every in-process
// request: the write rate limiter keys on it, and loopback is the honest
// answer — an /mcp caller is a loopback caller and shares the budget with
// the other loopback callers by design.
const inProcessRemoteAddr = "127.0.0.1:0"

// InProcessMaxResponseBytes caps one buffered response. It equals the
// wrapper client's own read bound (client.go reads at most 32 MiB), so the
// cap is reached here, before the bytes are copied a second time.
const InProcessMaxResponseBytes = 32 << 20

// ErrInProcessResponseTooLarge is returned by RoundTrip when the engine
// wrote more than InProcessMaxResponseBytes.
var ErrInProcessResponseTooLarge = errors.New("in-process response exceeds the buffer cap")

// EngineResolver returns the engine to serve a request with. It is called
// per request, so a rebuilt engine (ReassignAddress on desktop) is picked
// up without rebuilding the transport. An error means the API is not
// available (no account running); http.Client wraps it in *url.Error, which
// the wrapper reports as "unreachable".
type EngineResolver func() (http.Handler, error)

// NewInProcessTransport builds the transport over resolve.
func NewInProcessTransport(resolve EngineResolver) http.RoundTripper {
	return &inProcessTransport{resolve: resolve}
}

type inProcessTransport struct {
	resolve EngineResolver
}

// RoundTrip serves req in-process. The request is cloned before being
// handed to the engine (a RoundTripper must not modify its argument). The
// Authorization header is passed through exactly as the client set it.
//
// The engine runs on its own goroutine so the request's context can end
// the wait: a caller whose deadline passes gets the context's error back
// while the handler finishes on its own. A CallLease on the context is
// held until the handler actually returns, so whatever admission the
// lease stands for is not given back while the work it admitted still
// runs.
func (t *inProcessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := req.Context()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("in-process api: %w", err)
	}
	handler, err := t.resolve()
	if err != nil {
		return nil, fmt.Errorf("in-process api: %w", err)
	}
	served := req.Clone(ctx)
	served.RemoteAddr = inProcessRemoteAddr
	if served.Body == nil {
		served.Body = http.NoBody
	}
	rec := &bufferedResponse{header: http.Header{}, limit: InProcessMaxResponseBytes}
	lease := callLeaseFrom(ctx)
	if lease != nil {
		lease.hold()
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		if lease != nil {
			defer lease.Done()
		}
		defer func() {
			if p := recover(); p != nil {
				rec.panicked = fmt.Errorf("in-process handler panicked: %v", p)
			}
		}()
		handler.ServeHTTP(rec, served)
	}()
	select {
	case <-finished:
	case <-ctx.Done():
		return nil, fmt.Errorf("in-process api: %w", ctx.Err())
	}
	if rec.panicked != nil {
		return nil, rec.panicked
	}
	if rec.overflow {
		return nil, fmt.Errorf("in-process api: %w", ErrInProcessResponseTooLarge)
	}
	body := rec.body.Bytes()
	status := rec.status()
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rec.header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

// CallLease is held by everything one admitted call started: the /mcp
// handler holds it for the request, the in-process transport adds a hold
// for every inner handler it launches. The release runs once, when the
// last hold ends — so a caller that gave up on a deadline does not free
// its admission slot while its inner handler is still running.
type CallLease struct {
	mu      sync.Mutex
	holds   int
	release func()
}

// NewCallLease starts a lease with one hold, the caller's.
func NewCallLease(release func()) *CallLease {
	return &CallLease{holds: 1, release: release}
}

func (l *CallLease) hold() {
	l.mu.Lock()
	l.holds++
	l.mu.Unlock()
}

// Done ends one hold; the last one runs the release.
func (l *CallLease) Done() {
	l.mu.Lock()
	l.holds--
	var release func()
	if l.holds == 0 {
		release, l.release = l.release, nil
	}
	l.mu.Unlock()
	if release != nil {
		release()
	}
}

type callLeaseKey struct{}

// WithCallLease carries a lease on ctx for the transport to find.
func WithCallLease(ctx context.Context, l *CallLease) context.Context {
	return context.WithValue(ctx, callLeaseKey{}, l)
}

func callLeaseFrom(ctx context.Context) *CallLease {
	l, _ := ctx.Value(callLeaseKey{}).(*CallLease)
	return l
}

// bufferedResponse is the http.ResponseWriter the engine writes into:
// headers, one status, the whole body up to limit. Flush is a no-op — a
// buffered writer cannot stream, and this transport does not claim to.
type bufferedResponse struct {
	panicked error
	header   http.Header
	code     int
	body     bytes.Buffer
	limit    int
	overflow bool
}

func (r *bufferedResponse) Header() http.Header { return r.header }

func (r *bufferedResponse) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
}

func (r *bufferedResponse) Write(p []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	if r.overflow {
		return 0, ErrInProcessResponseTooLarge
	}
	if r.body.Len()+len(p) > r.limit {
		r.overflow = true
		return 0, ErrInProcessResponseTooLarge
	}
	return r.body.Write(p)
}

// Flush satisfies http.Flusher so a handler probing for it does not take a
// different code path; nothing is flushed, the body is served whole.
func (r *bufferedResponse) Flush() {}

func (r *bufferedResponse) status() int {
	if r.code == 0 {
		return http.StatusOK
	}
	return r.code
}
