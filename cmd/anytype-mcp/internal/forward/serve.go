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
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/contract"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/files"
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
	msgWithheld  = "Anytype's reply contained this connection's key and was withheld."
)

// Options configure one serve run.
type Options struct {
	ConnectionID string
	Store        store.Store
	In           io.Reader
	Out          io.Writer
	Log          io.Writer
	// Files is the folder of the full tier's local file tools, already
	// opened (serve --files-dir). Without it a full-tier connection gets a
	// private temporary folder of its own. Serve closes it.
	Files *files.Dir

	// Zero values take the contract's numbers; tests shorten them.
	RefreshAfter     time.Duration
	ExchangeTimeout  time.Duration
	ShutdownDeadline time.Duration
	StdoutStall      time.Duration
	ErrorBodyWait    time.Duration
	StoreTimeout     time.Duration
	TransferTimeout  time.Duration
	// FileGrace is how long a file transfer that was told to stop may
	// take to unwind before its worker answers without it.
	FileGrace time.Duration
	Now       func() time.Time

	// Test hooks. httpClient replaces the loopback client, so a test can
	// drop idle connections at a known moment; onServer hands out the
	// running server; afterAdmit runs once a request has passed the gate;
	// wrapFolder stands in front of the files folder.
	httpClient *http.Client
	onServer   func(*server)
	afterAdmit func(*call)
	wrapFolder func(folder) folder
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
	if o.StoreTimeout == 0 {
		o.StoreTimeout = contract.StoreReadTimeout
	}
	if o.TransferTimeout == 0 {
		o.TransferTimeout = contract.TransferTimeout
	}
	if o.FileGrace == 0 {
		o.FileGrace = time.Second
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

// writePiece is how much of a line goes to stdout per write call, so the
// stall timer sees progress on a large reply.
const writePiece = 64 << 10

// call is one accepted message on its way through a worker. canceled and
// committed are decided under server.mu: a request is either canceled
// before the writer turns to its reply, or its reply is written.
type call struct {
	msg       *message
	key       string // idKey of a request
	seq       uint64
	ctx       context.Context
	cancel    context.CancelFunc
	canceled  atomic.Bool
	committed bool
}

// session is one curated-tier conversation with heart, owned by the
// snapshot that opened it: its DELETE goes to that port with that key and
// nowhere else. On the full tier id is empty and only version is used.
type session struct {
	id      string
	version string
	snap    snapshot
}

type server struct {
	o    Options
	log  *logger
	conn *connection
	up   *upstream
	// local is the full tier's file tools; nil on a curated tier, without
	// a connection, and when no folder could be made
	local *localTools

	out        chan *reply
	queue      chan *call
	broken     chan struct{}
	brokenOnce sync.Once
	runCtx     context.Context
	accepted   sync.WaitGroup
	workers    sync.WaitGroup
	// progress is when stdout last took a piece of the line being
	// written, as a monotonic offset from epoch; zero while the writer is
	// idle. The stall timer never looks at the wall clock, which can jump.
	epoch    time.Time
	progress atomic.Int64

	renewMu sync.Mutex

	dropMu     sync.Mutex
	drop       chan session
	dropClosed bool
	dropDone   chan struct{}

	mu              sync.Mutex
	live            map[string]*call
	seq             uint64
	initialized     bool
	sess            session
	initParams      json.RawMessage
	resetPending    bool
	resetClearedSeq uint64
	renewals        int
}

// Serve runs the forwarder until stdin ends, ctx is canceled (SIGINT,
// SIGTERM) or stdout breaks, and returns the exit code. One deadline runs
// from the trigger to the return, whatever is still blocked: the drain,
// stdout, the session DELETEs and the log. The caller's os.Exit ends any
// goroutine that is still stuck.
func Serve(ctx context.Context, o Options) int {
	o.defaults()
	log := newLogger(o.Log)
	runCtx, stopRun := context.WithCancel(context.Background())
	defer stopRun()
	s := &server{
		o:        o,
		log:      log,
		conn:     &connection{id: o.ConnectionID, st: o.Store, now: o.Now, maxAge: o.RefreshAfter, timeout: o.StoreTimeout, log: log},
		up:       &upstream{client: o.httpClient, timeout: o.ExchangeTimeout, errBodyFor: o.ErrorBodyWait},
		out:      make(chan *reply, contract.QueueSize),
		queue:    make(chan *call, contract.QueueSize),
		broken:   make(chan struct{}),
		runCtx:   runCtx,
		epoch:    time.Now(),
		drop:     make(chan session, contract.QueueSize),
		dropDone: make(chan struct{}),
		live:     map[string]*call{},
	}
	if o.onServer != nil {
		o.onServer(s)
	}
	// the one read that decides this process's fate may stall (a network
	// home): a signal must still end the process, so the read runs beside
	// the wait. Nothing else starts before it returns: the reader and the
	// workers never run on an empty snapshot.
	started := make(chan struct{})
	go func() {
		defer close(started)
		s.conn.start(ctx)
	}()
	select {
	case <-started:
	case <-ctx.Done():
		log.printf("shutdown: signal before the item was read")
		deadline, cancel := context.WithTimeout(context.Background(), o.ShutdownDeadline)
		defer cancel()
		log.close(deadline)
		return 0
	}

	defer s.openFiles()()
	go s.writer()
	go s.watchStdout()
	go s.cleaner() //nolint:gosec // its DELETEs must outlive the run context: shutdown waits for them
	for i := 0; i < contract.Workers; i++ {
		s.workers.Add(1)
		go func() {
			defer s.workers.Done()
			s.worker()
		}()
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
		code = s.drain(ctx, deadline)
	case <-ctx.Done():
		deadline, cancel = context.WithTimeout(context.Background(), o.ShutdownDeadline)
		log.printf("shutdown: signal")
	case <-s.broken:
		deadline, cancel = context.WithTimeout(context.Background(), o.ShutdownDeadline)
		log.printf("shutdown: stdout is closed or stalled")
		code = 1
	}
	defer cancel()

	// the rest may block on heart or on the store: it gets what is left of
	// the deadline and is abandoned after it
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		stopRun()
		// a worker that already holds a session id from heart hands it to
		// cleanup before the cleanup queue closes
		idle := make(chan struct{})
		go func() {
			s.workers.Wait()
			close(idle)
		}()
		select {
		case <-idle:
		case <-deadline.Done():
			return
		}
		s.closeDrop()
		select {
		case <-s.dropDone:
		case <-deadline.Done():
			return
		}
		s.endSession(deadline)
		s.up.client.CloseIdleConnections()
	}()
	select {
	case <-finished:
	case <-deadline.Done():
	}
	log.close(deadline)
	return code
}

// drain finishes what was accepted before stdin ended, within the
// deadline, and returns the exit code: 1 when replies were lost.
func (s *server) drain(ctx, deadline context.Context) int {
	o, log, code := s.o, s.log, 0
	drained := make(chan struct{})
	go func() {
		s.accepted.Wait()
		close(drained)
	}()
	// a little before the deadline no file transfer starts any more, and
	// one still running is stopped, so that it can remove its part file in
	// time
	reserve := time.NewTimer(o.ShutdownDeadline - o.ShutdownDeadline/5)
	defer reserve.Stop()
	early := make(chan struct{})
	go func() {
		select {
		case <-reserve.C:
			// closed first, counted second: a transfer that starts now
			// either sees the door closed or is seen here
			if s.local != nil {
				s.local.closed.Store(true)
				if s.local.running.Load() > 0 {
					close(early)
				}
			}
		case <-drained:
		case <-deadline.Done():
		}
	}()
	select {
	case <-early:
		log.printf("shutdown: a file transfer is still running near the deadline")
		code = 1
	case <-drained:
		if !s.flush(deadline) {
			// replies were lost: this is not a clean exit
			code = 1
		}
	case <-deadline.Done():
		// their replies are lost: this is not a clean exit
		log.printf("shutdown: requests still running at the deadline")
		code = 1
	case <-ctx.Done():
	case <-s.broken:
		code = 1
	}
	if s.local != nil && s.local.dropped.Load() {
		// a transfer was turned away: its reply is lost
		code = 1
	}
	return code
}

// openFiles fixes the folder of the local file tools for the life of the
// process, on a full-tier connection only: the one the caller opened, else
// a new private temporary folder. It returns what releases it.
func (s *server) openFiles() func() {
	dir := s.o.Files
	release := func() {
		if dir != nil {
			_ = dir.Close()
		}
	}
	if s.conn.pinnedTier() != "full" {
		return release
	}
	if dir == nil {
		var err error
		if dir, err = files.Temp(); err != nil {
			s.log.printf("file tools are off: %v", err)
			dir = nil
			return release
		}
	}
	var where folder = dirFolder{dir}
	if s.o.wrapFolder != nil {
		where = s.o.wrapFolder(where)
	}
	local, err := newLocalTools(s, where, s.o.TransferTimeout)
	if err != nil {
		s.log.printf("file tools are off: %v", err)
		return release
	}
	s.local = local
	s.log.printf("file tools: folder %s", dir.Path())
	return release
}

// send queues a line that answers no accepted request (a refusal from the
// reader). It gives up only when stdout is broken or the run has stopped.
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

// sendFor queues the reply of an accepted request. A request canceled
// while its reply waits for room gives up the wait.
func (s *server) sendFor(c *call, r *reply) bool {
	r.call = c
	select {
	case s.out <- r:
		return true
	case <-c.ctx.Done():
	case <-s.broken:
	case <-s.runCtx.Done():
	}
	return false
}

// flush waits until everything queued so far is on stdout. It reports
// false when the deadline or a broken stdout got there first.
func (s *server) flush(ctx context.Context) bool {
	marker := &reply{flushed: make(chan struct{})}
	select {
	case s.out <- marker:
	case <-ctx.Done():
		return false
	case <-s.broken:
		return false
	}
	select {
	case <-marker.flushed:
		return true
	case <-ctx.Done():
	case <-s.broken:
	}
	return false
}

func (s *server) breakStdout() {
	s.brokenOnce.Do(func() { close(s.broken) })
}

// commit decides, at the moment the writer turns to a reply, whether it
// is written: not if its request was canceled before now; otherwise the
// request can no longer be canceled.
func (s *server) commit(c *call) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.live[c.key] == c {
		delete(s.live, c.key)
	}
	if c.canceled.Load() {
		return false
	}
	c.committed = true
	return true
}

// writer is the only goroutine that writes stdout: complete lines, one at
// a time, each in pieces so the stall timer measures progress.
func (s *server) writer() {
	for {
		select {
		case r := <-s.out:
			if r.flushed != nil {
				close(r.flushed)
				continue
			}
			if r.call != nil {
				ok := s.commit(r.call)
				r.call.cancel()
				if !ok {
					continue
				}
			}
			for line := r.line; len(line) > 0; {
				n := min(len(line), writePiece)
				s.progress.Store(s.elapsed())
				if _, err := s.o.Out.Write(line[:n]); err != nil {
					s.progress.Store(0)
					s.breakStdout()
					return
				}
				line = line[n:]
			}
			s.progress.Store(0)
			if r.after != nil {
				r.after()
			}
		case <-s.runCtx.Done():
			return
		}
	}
}

// elapsed is the monotonic time since the server started, never zero.
func (s *server) elapsed() int64 { return int64(time.Since(s.epoch)) + 1 }

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
			at := s.progress.Load()
			if at != 0 && time.Duration(s.elapsed()-at) >= s.o.StdoutStall {
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
		c.key = idKey(m.id)
		if _, dup := s.live[c.key]; dup {
			s.mu.Unlock()
			cancel()
			s.send(errorReply(m.id, codeInvalidRequest, "a request with this id is still running"))
			return
		}
		s.live[c.key] = c
	}
	s.mu.Unlock()

	s.accepted.Add(1)
	select {
	case s.queue <- c:
	default:
		s.forget(c)
		s.accepted.Done()
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

// forget ends a call that will write nothing.
func (s *server) forget(c *call) {
	c.cancel()
	if c.msg.kind != kindRequest {
		return
	}
	s.mu.Lock()
	if s.live[c.key] == c {
		delete(s.live, c.key)
	}
	s.mu.Unlock()
}

// cancelRequest serves the cancellation notification: the queued or
// running request is canceled and its reply suppressed, up to the moment
// the writer turns to that reply. It runs on the reader, so it works with
// a full queue.
func (s *server) cancelRequest(params json.RawMessage) {
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	key := idKey(p.RequestID)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.live[key]
	if c == nil || c.committed || c.msg.method == "initialize" {
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
	defer s.accepted.Done()
	if c.canceled.Load() {
		s.forget(c)
		return
	}
	if c.msg.kind != kindRequest {
		s.forwardQuiet(c)
		c.cancel()
		return
	}
	r := s.handle(c)
	if r == nil || c.canceled.Load() || s.runCtx.Err() != nil || !s.sendFor(c, r) {
		s.forget(c)
	}
}

// forwardQuiet passes a notification or a client response on. heart
// answers 202; nothing is written to stdout whatever happens. A refused
// connection sent nothing, so the item is re-read and the message tried
// once more.
func (s *server) forwardQuiet(c *call) {
	snap, err := s.conn.current(c.ctx)
	if err != nil {
		return
	}
	sess := s.session()
	ex := s.up.do(c.ctx, http.MethodPost, snap, sess.id, sess.version, c.msg.raw)
	if ex.out == outRefused {
		if snap, err = s.conn.refresh(c.ctx); err != nil {
			return
		}
		ex = s.up.do(c.ctx, http.MethodPost, snap, sess.id, sess.version, c.msg.raw)
	}
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

func (s *server) session() session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sess
}

// admit is the one locked step that reads the reset gate and the session
// together: a tools/call that passes the gate runs on the session it saw
// here for its whole life, never on a later one.
func (s *server) admit(c *call) (sess session, blocked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c.msg.method == "tools/call" && (s.resetPending || c.seq <= s.resetClearedSeq) {
		return session{}, true
	}
	return s.sess, false
}

// expiredReply is the session-reset error. The reset flag clears only
// once one of these lines is on stdout: the cut is taken after the write
// has returned, so every tools/call received before the client could have
// read the warning gets the warning and does not run. A call the client
// sends in the instant after reading it may get the warning once more;
// the text says the call did not run, so that is safe.
func (s *server) expiredReply(id json.RawMessage) *reply {
	r := toolErrorReply(id, msgExpired)
	r.after = func() {
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
	sess, blocked := s.admit(c)
	if blocked {
		return s.expiredReply(m.id)
	}
	if s.o.afterAdmit != nil {
		s.o.afterAdmit(c)
	}
	if isInit {
		sess = session{}
	}
	if isTool && s.local != nil {
		// heart wins a name it serves itself; the rest never reaches it
		if name, params, ok := s.local.claims(m.params); ok {
			return s.local.call(c, name, params)
		}
	}
	snap, err := s.conn.current(c.ctx)
	if err != nil {
		return s.notConnected(m.id)
	}
	refused, renewed := false, false
	for {
		ex := s.up.do(c.ctx, http.MethodPost, snap, sess.id, sess.version, m.raw)
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
			if sess.id == "" {
				return errorReply(m.id, codeMethodNotFound, msgTooOld)
			}
			if err := s.renew(c.ctx, sess.id); err != nil {
				return s.renewFailed(m, err)
			}
			if isTool {
				// never replayed, and never on the new session
				return s.expiredReply(m.id)
			}
			if renewed {
				return errorReply(m.id, codeInternal, msgNoAnswer)
			}
			renewed = true
			sess = s.session()
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

// carriesKey reports whether a body would put the key in front of the
// client: in its bytes as they are, or in any string the client would
// decode from it (a value or a member name), however JSON spells it. ok
// is false when the body can't be walked to its end; such a body is not
// relayed either.
func carriesKey(body []byte, key string) (hit, ok bool) {
	if key == "" {
		return false, true
	}
	if bytes.Contains(body, []byte(key)) {
		return true, true
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return false, true
		}
		if err != nil {
			return false, false
		}
		if str, isString := tok.(string); isString && strings.Contains(str, key) {
			return true, true
		}
	}
}

// relay turns a 200 into the reply. Only a JSON-RPC response carrying this
// request's id is relayed; anything else leaves the outcome unconfirmed.
// A body that contains the connection's key is never relayed: the reply
// would end up in the AI app's transcript.
func (s *server) relay(c *call, snap snapshot, ex exchange) *reply {
	m := c.msg
	isInit := m.method == "initialize"
	adopted := false
	defer func() {
		// a failed initialize ends any session id it received
		if isInit && !adopted && ex.session != "" {
			s.dropSession(session{id: ex.session, snap: snap})
		}
	}()
	if ex.bodyErr != nil || !ex.isJSON {
		s.log.printf("%s: unreadable reply", m.method)
		return unconfirmed(m)
	}
	hit, walked := carriesKey(ex.body, snap.key)
	if !walked {
		s.log.printf("%s: unreadable reply", m.method)
		return unconfirmed(m)
	}
	if hit {
		s.log.printf("%s: the reply contained the connection's key and was withheld", m.method)
		if m.method == "tools/call" {
			return toolErrorReply(m.id, msgWithheld)
		}
		return errorReply(m.id, codeInternal, msgWithheld)
	}
	rpc, ok := parseRPC(ex.body)
	if !ok || idKey(rpc.ID) == "" || idKey(rpc.ID) != c.key {
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
	if m.method == "tools/list" && s.local != nil {
		result = s.local.extend(m.params, result)
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
	old := s.sess
	s.sess = session{id: ex.session, version: heartVersion, snap: snap}
	s.initParams = append(json.RawMessage(nil), m.params...)
	s.initialized = true
	// resetPending stays as it is: only the written warning clears the
	// gate, or a tools/call sent before the client read the warning could
	// run on this new session with targets from the old one
	s.mu.Unlock()
	if old.id != "" && old.id != ex.session {
		// a replaced initialize ends the session it replaced
		s.dropSession(old)
	}
	return rewritten, nil
}

var (
	errRenewTooOld   = errors.New("no /mcp route")
	errRenewFailed   = errors.New("renewal failed")
	errRenewRejected = errors.New("key rejected")
)

func (s *server) renewFailed(m *message, err error) *reply {
	switch {
	case errors.Is(err, errNotConnected):
		return s.notConnected(m.id)
	case errors.Is(err, errRenewRejected):
		return errorReply(m.id, codeNotConnected, msgRejected)
	case errors.Is(err, errRenewTooOld):
		return errorReply(m.id, codeMethodNotFound, msgTooOld)
	case m.method == "tools/call":
		return toolErrorReply(m.id, msgNoRenew)
	}
	return errorReply(m.id, codeInternal, msgNoAnswer)
}

// renew opens a new session after heart answered 404 for stale. It is
// shared: requests that hit the same 404 wait here and find the work done.
// Both legs are awaited and checked like any other exchange: the
// initialize must be answered in JSON under the id it was sent with, and
// the notification with 202. The new session is installed only if stale
// is still the current one; a client initialize that got in first keeps
// its session, and the one opened here is ended.
func (s *server) renew(ctx context.Context, stale string) error {
	s.renewMu.Lock()
	defer s.renewMu.Unlock()
	s.mu.Lock()
	current, params := s.sess.id, s.initParams
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
	id, err := json.Marshal(fmt.Sprintf("anytype-mcp-renew-%d", n))
	if err != nil {
		return errRenewFailed
	}
	req := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "method": "initialize"}
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
	opened := session{id: ex.session, snap: snap}
	fail := func(status int, err error) error {
		if opened.id != "" {
			s.dropSession(opened)
		}
		s.log.printf("session renewal failed: HTTP %d", status)
		return err
	}
	rejected := func(status int) error {
		if _, err := s.conn.refresh(ctx); err != nil {
			return fail(status, err)
		}
		return fail(status, errRenewRejected)
	}
	switch {
	case ex.status == http.StatusNotFound:
		return fail(ex.status, errRenewTooOld)
	case ex.status == http.StatusUnauthorized:
		return rejected(ex.status)
	case ex.status != http.StatusOK || ex.bodyErr != nil || !ex.isJSON || ex.session == "":
		return fail(ex.status, errRenewFailed)
	}
	rpc, ok := parseRPC(ex.body)
	hit, walked := carriesKey(ex.body, snap.key)
	if !ok || rpc.Error != nil || idKey(rpc.ID) != idKey(id) || hit || !walked {
		return fail(ex.status, errRenewFailed)
	}
	var res struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if json.Unmarshal(rpc.Result, &res) != nil || res.ProtocolVersion == "" {
		return fail(ex.status, errRenewFailed)
	}
	opened.version = res.ProtocolVersion
	// the first leg may have taken a long time: the second is sent only
	// if the item still names the port and key the session was opened on
	if !s.stillHome(ctx, snap) {
		return fail(ex.status, errRenewFailed)
	}
	note := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	ack := s.up.do(ctx, http.MethodPost, snap, opened.id, opened.version, note)
	switch {
	case ack.out != outReply:
		return fail(0, errRenewFailed)
	case ack.status == http.StatusUnauthorized:
		return rejected(ack.status)
	case ack.status != http.StatusAccepted:
		return fail(ack.status, errRenewFailed)
	}
	s.mu.Lock()
	if s.sess.id != stale {
		// a client initialize installed its own session meanwhile
		s.mu.Unlock()
		s.dropSession(opened)
		return nil
	}
	s.sess = opened
	s.resetPending = true
	s.mu.Unlock()
	s.log.printf("session renewed")
	return nil
}

// dropSession hands a session nobody will use again to the cleaner. It
// never waits: a full queue leaves the session to heart's idle timeout.
func (s *server) dropSession(sess session) {
	if sess.id == "" {
		return
	}
	s.dropMu.Lock()
	defer s.dropMu.Unlock()
	if s.dropClosed {
		return
	}
	select {
	case s.drop <- sess:
	default:
		s.log.printf("session cleanup queue full: left to expire")
	}
}

func (s *server) closeDrop() {
	s.dropMu.Lock()
	defer s.dropMu.Unlock()
	if !s.dropClosed {
		s.dropClosed = true
		close(s.drop)
	}
}

// stillHome reports whether the item still names the port and key of
// snap, re-reading it when the last read is old. Every authenticated
// follow-up on a session (a DELETE, the renewal's notification) is sent
// only while this holds: the bearer never goes to a port Anytype has left,
// where another user's program may now listen.
func (s *server) stillHome(ctx context.Context, snap snapshot) bool {
	cur, ok := s.conn.confirmed(ctx)
	return ok && cur.port == snap.port && cur.key == snap.key
}

// cleaner ends dropped sessions one at a time, each with the snapshot that
// opened it and only while the item still names that snapshot's port and
// key; otherwise heart's idle timeout ends the session. Cleanup adds one
// call to the four workers and stays under heart's limit per key. It
// outlives the run: shutdown closes the queue and waits for it within the
// deadline.
func (s *server) cleaner() {
	defer close(s.dropDone)
	for sess := range s.drop {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if s.stillHome(ctx, sess.snap) {
			s.up.do(ctx, http.MethodDelete, sess.snap, sess.id, sess.version, nil)
		} else {
			s.log.printf("session cleanup: its Anytype has moved; left to expire")
		}
		cancel()
	}
}

// endSession makes the one DELETE attempt of shutdown, and only where the
// session lives: the item is re-read if the last read is old, and the
// DELETE is sent only if the item still names the port and key the session
// was opened with. Otherwise heart's idle timeout ends it; the bearer is
// never sent to a port Anytype has left.
func (s *server) endSession(ctx context.Context) {
	sess := s.session()
	if sess.id == "" {
		return
	}
	if !s.stillHome(ctx, sess.snap) {
		s.log.printf("shutdown: the session's Anytype has moved; left to expire")
		return
	}
	s.up.do(ctx, http.MethodDelete, sess.snap, sess.id, sess.version, nil)
}
