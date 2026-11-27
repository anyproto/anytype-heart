package forward

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/contract"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

// Texts a person or a model reads.
const (
	msgReconnect = "Anytype is not connected to this app. Open Anytype ▸ Settings ▸ AI apps and reconnect, then restart this app."
	msgRejected  = "Anytype no longer accepts this connection's key. Open Anytype ▸ Settings ▸ AI apps and reconnect."
	msgStart     = "Anytype is not running. Start Anytype and try again."
	msgTooOld    = "This Anytype is too old for this connection. Update Anytype."
	msgBusy      = "The connection is busy: too many requests are waiting. Try again."
	msgForbidden = "This connection's key does not allow that. Change its access in Anytype ▸ Settings ▸ AI apps."
	msgHeartBusy = "Anytype is busy with other calls from this connection. Try again in a moment."
	msgExpired   = "The Anytype session expired and a new one was started. Anything remembered from earlier calls is gone: re-read your targets, then call again. This call did not run."
	msgNoRenew   = "The Anytype session expired and could not be renewed. This call did not run. Try again."
	msgUnknown   = "Anytype did not confirm the outcome of this call. It may or may not have been applied: check before retrying."
	msgNoAnswer  = "Anytype did not answer this request."
	msgNotInit   = "Send initialize first."
	msgTooLarge  = "The message is larger than 8 MiB."
)

// Options configure one serve run.
type Options struct {
	ConnectionID string
	Store        store.Store
	In           io.Reader
	Out          io.Writer
	Log          io.Writer

	// PortOverride and TierOverride replace the item's values. Only the
	// test build sets them.
	PortOverride int
	TierOverride string

	// Zero values take the contract's numbers; tests shorten them.
	RefreshAfter     time.Duration
	ExchangeTimeout  time.Duration
	ShutdownDeadline time.Duration
	StdoutStall      time.Duration
	ErrorBodyWait    time.Duration
	Now              func() time.Time

	// httpClient replaces the loopback client; tests use it to drop idle
	// connections at a known moment.
	httpClient *http.Client
}

func (o *Options) defaults() {
	if o.RefreshAfter == 0 {
		o.RefreshAfter = contract.RefreshAfter
	}
	if o.ExchangeTimeout == 0 {
		o.ExchangeTimeout = contract.ExchangeTimeout
	}
	if o.ShutdownDeadline == 0 {
		o.ShutdownDeadline = contract.ShutdownDeadline
	}
	if o.StdoutStall == 0 {
		o.StdoutStall = contract.StdoutStall
	}
	if o.ErrorBodyWait == 0 {
		o.ErrorBodyWait = 5 * time.Second
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.httpClient == nil {
		o.httpClient = newHTTPClient()
	}
}

// methodCancel is the cancellation notification, in the protocol's own
// spelling.
const methodCancel = "notifications/cancelled" //nolint:misspell

// call is one accepted message on its way through a worker.
type call struct {
	msg      *message
	seq      uint64
	ctx      context.Context
	cancel   context.CancelFunc
	canceled atomic.Bool
}

type server struct {
	o    Options
	log  *logger
	conn *connection
	up   *upstream

	out        chan *reply
	queue      chan *call
	broken     chan struct{}
	brokenOnce sync.Once
	runCtx     context.Context
	accepted   sync.WaitGroup
	writeBegan atomic.Int64

	renewMu sync.Mutex

	mu              sync.Mutex
	live            map[string]*call
	seq             uint64
	initialized     bool
	session         string
	heartVersion    string
	initParams      json.RawMessage
	resetPending    bool
	resetClearedSeq uint64
	renewals        int
}

// Serve runs the forwarder until stdin ends, ctx is canceled (SIGINT,
// SIGTERM) or stdout breaks, and returns the exit code. One deadline runs
// from the trigger to the return: the drain, stdout and one DELETE of a
// known session.
func Serve(ctx context.Context, o Options) int {
	o.defaults()
	log := &logger{w: o.Log}
	runCtx, stopRun := context.WithCancel(context.Background())
	defer stopRun()
	s := &server{
		o:   o,
		log: log,
		conn: &connection{id: o.ConnectionID, st: o.Store, now: o.Now, maxAge: o.RefreshAfter,
			portOver: o.PortOverride, tierOver: o.TierOverride, log: log},
		up:     &upstream{client: o.httpClient, timeout: o.ExchangeTimeout, errBodyFor: o.ErrorBodyWait},
		out:    make(chan *reply, contract.QueueSize),
		queue:  make(chan *call, contract.QueueSize),
		broken: make(chan struct{}),
		runCtx: runCtx,
		live:   map[string]*call{},
	}
	s.conn.start(ctx)

	go s.writer()
	go s.watchStdout()
	for i := 0; i < contract.Workers; i++ {
		go s.worker()
	}
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		s.reader()
	}()

	code := 0
	var deadline context.Context
	var cancel context.CancelFunc
	select {
	case <-readerDone:
		// stdin ended: finish what was accepted, within the deadline
		deadline, cancel = context.WithTimeout(context.Background(), o.ShutdownDeadline)
		drained := make(chan struct{})
		go func() {
			s.accepted.Wait()
			close(drained)
		}()
		select {
		case <-drained:
			s.flush(deadline)
		case <-deadline.Done():
			log.printf("shutdown: requests still running at the deadline")
		case <-ctx.Done():
		case <-s.broken:
			code = 1
		}
	case <-ctx.Done():
		deadline, cancel = context.WithTimeout(context.Background(), o.ShutdownDeadline)
		log.printf("shutdown: signal")
	case <-s.broken:
		deadline, cancel = context.WithTimeout(context.Background(), o.ShutdownDeadline)
		log.printf("shutdown: stdout is closed or stalled")
		code = 1
	}
	defer cancel()
	stopRun()
	s.endSession(deadline)
	s.up.client.CloseIdleConnections()
	return code
}

// send queues a line for stdout. It gives up only when stdout is broken or
// the run has stopped.
func (s *server) send(r *reply) {
	if r == nil {
		return
	}
	select {
	case s.out <- r:
	case <-s.broken:
	case <-s.runCtx.Done():
	}
}

func (s *server) flush(ctx context.Context) {
	marker := &reply{flushed: make(chan struct{})}
	s.send(marker)
	select {
	case <-marker.flushed:
	case <-ctx.Done():
	case <-s.broken:
	}
}

func (s *server) breakStdout() {
	s.brokenOnce.Do(func() { close(s.broken) })
}

// writer is the only goroutine that writes stdout: complete lines, one at
// a time.
func (s *server) writer() {
	for {
		select {
		case r := <-s.out:
			if r.flushed != nil {
				close(r.flushed)
				continue
			}
			if r.before != nil {
				r.before()
			}
			s.writeBegan.Store(s.o.Now().UnixNano())
			_, err := s.o.Out.Write(r.line)
			s.writeBegan.Store(0)
			if err != nil {
				s.breakStdout()
				return
			}
		case <-s.runCtx.Done():
			return
		}
	}
}

// watchStdout treats a write that makes no progress for the stall window
// as a broken stdout.
func (s *server) watchStdout() {
	every := s.o.StdoutStall / 4
	if every <= 0 {
		every = time.Millisecond
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			began := s.writeBegan.Load()
			if began != 0 && s.o.Now().Sub(time.Unix(0, began)) >= s.o.StdoutStall {
				s.breakStdout()
				return
			}
		case <-s.runCtx.Done():
			return
		case <-s.broken:
			return
		}
	}
}

// reader validates and classifies each line and never waits for heart.
func (s *server) reader() {
	br := bufio.NewReaderSize(s.o.In, 64<<10)
	for {
		line, err := readLine(br, contract.MaxMessageBytes)
		if errors.Is(err, errLineTooLong) {
			s.send(errorReply(nullID, codeInvalidRequest, msgTooLarge))
			continue
		}
		if len(bytes.TrimSpace(line)) > 0 {
			if msg, refusal := parseEnvelope(line); refusal != nil {
				s.send(refusal)
			} else {
				s.dispatch(msg)
			}
		}
		if err != nil {
			return
		}
		select {
		case <-s.runCtx.Done():
			return
		default:
		}
	}
}

func (s *server) dispatch(m *message) {
	if m.kind == kindNotification && m.method == methodCancel {
		s.cancelRequest(m.params)
		return
	}
	connected := s.conn.connected()
	s.mu.Lock()
	initialized := s.initialized
	s.mu.Unlock()

	if m.kind != kindRequest {
		// notifications and client responses: forwarded once there is a
		// conversation to forward them into; never answered
		if !connected || !initialized {
			return
		}
		s.enqueue(m)
		return
	}
	switch {
	case !connected:
		s.send(errorReply(m.id, codeNotConnected, msgReconnect))
		return
	case !initialized && m.method == "ping":
		s.send(mustResult(m.id))
		return
	case !initialized && m.method != "initialize":
		s.send(errorReply(m.id, codeNotInitialized, msgNotInit))
		return
	}
	s.enqueue(m)
}

func mustResult(id json.RawMessage) *reply {
	r, err := resultReply(id, json.RawMessage("{}"))
	if err != nil {
		return errorReply(nullID, codeInternal, "internal error")
	}
	return r
}

// enqueue hands a message to the workers without waiting. A request that
// finds the queue full is answered at once; anything else is dropped.
func (s *server) enqueue(m *message) {
	ctx, cancel := context.WithCancel(s.runCtx)
	c := &call{msg: m, ctx: ctx, cancel: cancel}
	s.mu.Lock()
	s.seq++
	c.seq = s.seq
	if m.kind == kindRequest {
		if _, dup := s.live[string(m.id)]; dup {
			s.mu.Unlock()
			cancel()
			s.send(errorReply(m.id, codeInvalidRequest, "a request with this id is still running"))
			return
		}
		s.live[string(m.id)] = c
	}
	s.mu.Unlock()

	s.accepted.Add(1)
	select {
	case s.queue <- c:
	default:
		s.release(c)
		if m.kind == kindRequest {
			s.send(errorReply(m.id, codeBusy, msgBusy))
		} else {
			s.log.printf("queue full: dropped a %s", describeKind(m))
		}
	}
}

func describeKind(m *message) string {
	if m.kind == kindResponse {
		return "client response"
	}
	return "notification"
}

func (s *server) release(c *call) {
	c.cancel()
	if c.msg.kind == kindRequest {
		s.mu.Lock()
		if s.live[string(c.msg.id)] == c {
			delete(s.live, string(c.msg.id))
		}
		s.mu.Unlock()
	}
	s.accepted.Done()
}

// cancelRequest serves the cancellation notification: the queued or running
// request is canceled and its reply suppressed. It runs on the reader, so
// it works with a full queue.
func (s *server) cancelRequest(params json.RawMessage) {
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(params, &p) != nil || len(p.RequestID) == 0 {
		return
	}
	s.mu.Lock()
	c := s.live[string(bytes.TrimSpace(p.RequestID))]
	s.mu.Unlock()
	if c == nil || c.msg.method == "initialize" {
		// initialize can't be canceled (MCP lifecycle)
		return
	}
	c.canceled.Store(true)
	c.cancel()
}

func (s *server) worker() {
	for {
		select {
		case c := <-s.queue:
			s.run(c)
		case <-s.runCtx.Done():
			return
		}
	}
}

func (s *server) run(c *call) {
	defer s.release(c)
	if c.canceled.Load() {
		return
	}
	if c.msg.kind != kindRequest {
		s.forwardQuiet(c)
		return
	}
	r := s.handle(c)
	if r == nil || c.canceled.Load() || s.runCtx.Err() != nil {
		return
	}
	s.send(r)
}

// forwardQuiet passes a notification or a client response on. heart
// answers 202; nothing is written to stdout whatever happens.
func (s *server) forwardQuiet(c *call) {
	snap, err := s.conn.current(c.ctx)
	if err != nil {
		return
	}
	session, version := s.sessionState()
	ex := s.up.do(c.ctx, http.MethodPost, snap, session, version, c.msg.raw)
	switch {
	case ex.out != outReply:
		s.log.printf("%s not delivered", describeKind(c.msg))
	case ex.status == http.StatusUnauthorized:
		if _, err := s.conn.refresh(c.ctx); err != nil {
			s.log.printf("%s refused: the connection is gone", describeKind(c.msg))
		}
	case ex.status >= 300:
		s.log.printf("%s refused: HTTP %d", describeKind(c.msg), ex.status)
	}
}

func (s *server) sessionState() (session, version string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session, s.heartVersion
}

func (s *server) resetBlocks(seq uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resetPending || seq <= s.resetClearedSeq
}

// expiredReply is the session-reset error. The reset flag clears when
// the writer turns to one of these lines; every tools/call received before
// that moment gets the same error and does not run. The cut is taken
// before the write, not after it: a client that has read the warning may
// answer before the write call returns here, and that answer must run. If
// the write then fails, stdout is broken and the process ends.
func (s *server) expiredReply(id json.RawMessage) *reply {
	r := toolErrorReply(id, msgExpired)
	r.before = func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.resetPending {
			s.resetPending = false
			s.resetClearedSeq = s.seq
		}
	}
	return r
}

func (s *server) notConnected(id json.RawMessage) *reply {
	return errorReply(id, codeNotConnected, msgReconnect)
}

// unconfirmed answers a request whose outcome nobody can know.
func unconfirmed(m *message) *reply {
	if m.method == "tools/call" {
		return toolErrorReply(m.id, msgUnknown)
	}
	return errorReply(m.id, codeInternal, msgNoAnswer)
}

// handle runs one request to its single reply (nil when it was canceled).
func (s *server) handle(c *call) *reply {
	m := c.msg
	isTool := m.method == "tools/call"
	isInit := m.method == "initialize"
	if isTool && s.resetBlocks(c.seq) {
		return s.expiredReply(m.id)
	}
	snap, err := s.conn.current(c.ctx)
	if err != nil {
		return s.notConnected(m.id)
	}
	refused, renewed := false, false
	for {
		session, version := "", ""
		if !isInit {
			session, version = s.sessionState()
		}
		ex := s.up.do(c.ctx, http.MethodPost, snap, session, version, m.raw)
		if c.canceled.Load() && !isInit {
			return nil
		}
		switch ex.out {
		case outRefused:
			// nothing was sent: re-read the item and try once more
			next, err := s.conn.refresh(c.ctx)
			if err != nil {
				return s.notConnected(m.id)
			}
			if !refused {
				refused = true
				snap = next
				continue
			}
			return errorReply(m.id, codeNotConnected, msgStart)
		case outLost:
			s.log.printf("%s: no reply from Anytype", m.method)
			return unconfirmed(m)
		}

		switch st := ex.status; {
		case st == http.StatusOK:
			return s.relay(c, snap, ex)
		case st == http.StatusUnauthorized:
			// the request failed and is never replayed; the next one runs
			// on whatever the item says now
			if _, err := s.conn.refresh(c.ctx); err != nil {
				return s.notConnected(m.id)
			}
			return errorReply(m.id, codeNotConnected, msgRejected)
		case st == http.StatusForbidden:
			if isTool {
				return toolErrorReply(m.id, msgForbidden)
			}
			return errorReply(m.id, codeNotConnected, msgForbidden)
		case st == http.StatusNotFound:
			if session == "" {
				return errorReply(m.id, codeMethodNotFound, msgTooOld)
			}
			if err := s.renew(c.ctx, session); err != nil {
				return s.renewFailed(m, err)
			}
			if isTool {
				return s.expiredReply(m.id)
			}
			if renewed {
				return errorReply(m.id, codeInternal, msgNoAnswer)
			}
			renewed = true
			if snap, err = s.conn.current(c.ctx); err != nil {
				return s.notConnected(m.id)
			}
			continue
		case st == http.StatusBadRequest || st == http.StatusRequestTimeout || st == http.StatusRequestEntityTooLarge:
			if rpc, ok := parseRPC(ex.body); ok && st == http.StatusBadRequest && rpc.Error != nil {
				// heart's own JSON-RPC refusal, whatever id it carries;
				// its data member is dropped
				return errorReply(m.id, rpc.code, redact(rpc.message, snap.key))
			}
			if msg, ok := restMessage(ex.body); ok {
				return errorReply(m.id, codeInvalidRequest, redact(msg, snap.key))
			}
			return errorReply(m.id, codeInvalidRequest, fmt.Sprintf("Anytype refused the request (HTTP %d).", st))
		case st == http.StatusTooManyRequests:
			if isTool {
				return toolErrorReply(m.id, msgHeartBusy)
			}
			return errorReply(m.id, codeInternal, msgHeartBusy)
		case st >= 500:
			s.log.printf("%s: HTTP %d", m.method, st)
			text := fmt.Sprintf("Anytype failed to handle this (HTTP %d).", st)
			if msg, ok := restMessage(ex.body); ok {
				text = fmt.Sprintf("Anytype failed to handle this (HTTP %d): %s", st, redact(msg, snap.key))
			}
			if isTool {
				return toolErrorReply(m.id, text)
			}
			return errorReply(m.id, codeInternal, text)
		default:
			// 202 for a request, a redirect, anything else unexpected
			s.log.printf("%s: unexpected HTTP %d", m.method, st)
			return unconfirmed(m)
		}
	}
}

// relay turns a 200 into the reply. Only a JSON-RPC response carrying this
// request's id is relayed; anything else leaves the outcome unconfirmed.
func (s *server) relay(c *call, snap snapshot, ex exchange) *reply {
	m := c.msg
	isInit := m.method == "initialize"
	adopted := false
	defer func() {
		// a failed initialize ends any session id it received
		if isInit && !adopted && ex.session != "" {
			go s.dropSession(snap, ex.session, "")
		}
	}()
	if ex.bodyErr != nil || !ex.isJSON {
		s.log.printf("%s: unreadable reply", m.method)
		return unconfirmed(m)
	}
	rpc, ok := parseRPC(ex.body)
	if !ok || !bytes.Equal(bytes.TrimSpace(rpc.ID), m.id) {
		s.log.printf("%s: reply is not this request's", m.method)
		return unconfirmed(m)
	}
	if rpc.Error != nil {
		r, err := encode(wireReply{ID: m.id, Error: rpc.Error})
		if err != nil {
			return unconfirmed(m)
		}
		return r
	}
	result := rpc.Result
	if isInit {
		rewritten, err := s.adoptInitialize(m, snap, ex, result)
		if err != nil {
			s.log.printf("initialize: %v", err)
			return errorReply(m.id, codeInternal, msgNoAnswer)
		}
		adopted = true
		result = rewritten
	}
	r, err := resultReply(m.id, result)
	if err != nil {
		return unconfirmed(m)
	}
	return r
}

// negotiate picks the version the client sees: its own if known, else the
// newest known one below it, else heart's.
func negotiate(requested, heart string) string {
	best := ""
	for _, v := range contract.ClientVersions {
		if v == requested {
			return v
		}
		if v < requested {
			best = v
		}
	}
	if best != "" {
		return best
	}
	return heart
}

// adoptInitialize records a successful initialize: the session heart
// opened, the version it answered and the params to renew with. It returns
// the result with the client's version in it.
func (s *server) adoptInitialize(m *message, snap snapshot, ex exchange, result json.RawMessage) (json.RawMessage, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(result, &members); err != nil {
		return nil, errors.New("result is not an object")
	}
	var heartVersion string
	if err := json.Unmarshal(members["protocolVersion"], &heartVersion); err != nil || heartVersion == "" {
		return nil, errors.New("result has no protocolVersion")
	}
	if snap.tier != "full" && ex.session == "" {
		return nil, errors.New("no session id on a curated tier")
	}
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(m.params) > 0 {
		if err := json.Unmarshal(m.params, &p); err != nil {
			// params of another shape name no version: heart's is used
			p.ProtocolVersion = ""
		}
	}
	seen, err := json.Marshal(negotiate(p.ProtocolVersion, heartVersion))
	if err != nil {
		return nil, err
	}
	members["protocolVersion"] = seen
	rewritten, err := json.Marshal(members)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	old := s.session
	s.session = ex.session
	s.heartVersion = heartVersion
	s.initParams = append(json.RawMessage(nil), m.params...)
	s.initialized = true
	s.resetPending = false
	s.mu.Unlock()
	if old != "" && old != ex.session {
		// a replaced initialize ends the session it replaced
		go s.dropSession(snap, old, heartVersion)
	}
	return rewritten, nil
}

var (
	errRenewTooOld = errors.New("no /mcp route")
	errRenewFailed = errors.New("renewal failed")
)

func (s *server) renewFailed(m *message, err error) *reply {
	switch {
	case errors.Is(err, errNotConnected):
		return s.notConnected(m.id)
	case errors.Is(err, errRenewTooOld):
		return errorReply(m.id, codeMethodNotFound, msgTooOld)
	case m.method == "tools/call":
		return toolErrorReply(m.id, msgNoRenew)
	}
	return errorReply(m.id, codeInternal, msgNoAnswer)
}

// renew opens a new session after heart answered 404 for stale. It is
// shared: requests that hit the same 404 wait here and find the work done.
// Both the initialize and notifications/initialized are awaited.
func (s *server) renew(ctx context.Context, stale string) error {
	s.renewMu.Lock()
	defer s.renewMu.Unlock()
	s.mu.Lock()
	current, params := s.session, s.initParams
	s.renewals++
	n := s.renewals
	s.mu.Unlock()
	if current != stale {
		return nil
	}
	snap, err := s.conn.current(ctx)
	if err != nil {
		return err
	}
	req := map[string]any{"jsonrpc": "2.0", "id": fmt.Sprintf("anytype-mcp-renew-%d", n), "method": "initialize"}
	if len(params) > 0 {
		req["params"] = params
	}
	body, err := json.Marshal(req)
	if err != nil {
		return errRenewFailed
	}
	ex := s.up.do(ctx, http.MethodPost, snap, "", "", body)
	if ex.out != outReply {
		return errRenewFailed
	}
	fail := func(err error) error {
		if ex.session != "" {
			go s.dropSession(snap, ex.session, "")
		}
		s.log.printf("session renewal failed: HTTP %d", ex.status)
		return err
	}
	switch {
	case ex.status == http.StatusNotFound:
		return fail(errRenewTooOld)
	case ex.status == http.StatusUnauthorized:
		if _, err := s.conn.refresh(ctx); err != nil {
			return fail(err)
		}
		return fail(errRenewFailed)
	case ex.status != http.StatusOK || ex.bodyErr != nil || ex.session == "":
		return fail(errRenewFailed)
	}
	rpc, ok := parseRPC(ex.body)
	if !ok || rpc.Error != nil {
		return fail(errRenewFailed)
	}
	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(rpc.Result, &res) != nil || res.ProtocolVersion == "" {
		return fail(errRenewFailed)
	}
	note := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if ack := s.up.do(ctx, http.MethodPost, snap, ex.session, res.ProtocolVersion, note); ack.out != outReply || ack.status >= 300 {
		return fail(errRenewFailed)
	}
	s.mu.Lock()
	s.session = ex.session
	s.heartVersion = res.ProtocolVersion
	s.resetPending = true
	s.mu.Unlock()
	s.log.printf("session renewed")
	return nil
}

// dropSession ends a session nobody will use again.
func (s *server) dropSession(snap snapshot, session, version string) {
	ctx, cancel := context.WithTimeout(s.runCtx, 5*time.Second)
	defer cancel()
	s.up.do(ctx, http.MethodDelete, snap, session, version, nil)
}

// endSession makes the one DELETE attempt of shutdown.
func (s *server) endSession(ctx context.Context) {
	session, version := s.sessionState()
	if session == "" {
		return
	}
	s.conn.mu.Lock()
	snap, dead := s.conn.snap, s.conn.dead
	s.conn.mu.Unlock()
	if dead != "" {
		return
	}
	s.up.do(ctx, http.MethodDelete, snap, session, version, nil)
}
