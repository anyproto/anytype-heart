package forward

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/contract"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/files"
	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

const canaryKey = "CANARY-key-0123456789abcdefghijklmnopqrstuv"

type recorded struct {
	Method  string
	Path    string
	Session string
	Version string
	Auth    string
	Body    string
	RPC     string // JSON-RPC method of the body
	Query   string
	Header  http.Header
}

// fakeFile is a file object heart holds: name is its stem and extension
// joined, as the object's properties split them.
type fakeFile struct {
	name      string
	mediaType string
	bytes     []byte
}

// fakeHeart mimics core/api/server/mcp.go closely enough for the
// forwarder's contract: bearer check, curated-tier sessions, 404 on an
// unknown session, 202 for notifications, JSON-only replies.
type fakeHeart struct {
	t   *testing.T
	srv *httptest.Server

	mu       sync.Mutex
	key      string
	sessions map[string]bool
	opened   int
	tools    []any
	files    map[string]fakeFile
	reqs     []recorded
	// stored counts uploads heart received whole and stored
	stored int
	// hook may take a request over; it returns true when it answered.
	hook func(w http.ResponseWriter, r *http.Request, rec recorded) bool
}

func newFakeHeart(t *testing.T) *fakeHeart {
	h := &fakeHeart{t: t, key: canaryKey, sessions: map[string]bool{}}
	h.srv = httptest.NewServer(http.HandlerFunc(h.serve))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *fakeHeart) port() int {
	_, p, _ := net.SplitHostPort(h.srv.Listener.Addr().String())
	n, _ := strconv.Atoi(p)
	return n
}

func (h *fakeHeart) setHook(f func(w http.ResponseWriter, r *http.Request, rec recorded) bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hook = f
}

func (h *fakeHeart) requests() []recorded {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]recorded(nil), h.reqs...)
}

func (h *fakeHeart) count(rpc string) int {
	n := 0
	for _, r := range h.requests() {
		if r.RPC == rpc {
			n++
		}
	}
	return n
}

// setKey makes heart take another key than the one it took so far.
func (h *fakeHeart) setKey(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.key = key
}

func (h *fakeHeart) storedUploads() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.stored
}

func (h *fakeHeart) expireSessions() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sessions = map[string]bool{}
}

func rest(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"object": "error", "status": status, "code": "x", "message": msg})
}

func rpcResult(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (h *fakeHeart) serve(w http.ResponseWriter, r *http.Request) {
	body, readErr := io.ReadAll(r.Body)
	var msg struct {
		ID     json.RawMessage `json:"id"`
		Method string          `json:"method"`
		Params struct {
			ProtocolVersion string `json:"protocolVersion"`
		} `json:"params"`
	}
	_ = json.Unmarshal(body, &msg)
	rec := recorded{Method: r.Method, Path: r.URL.Path, Session: r.Header.Get(contract.SessionHeader),
		Version: r.Header.Get(contract.ProtocolHeader), Auth: r.Header.Get("Authorization"), Body: string(body), RPC: msg.Method,
		Query: r.URL.RawQuery, Header: r.Header.Clone()}
	h.mu.Lock()
	h.reqs = append(h.reqs, rec)
	hook, key := h.hook, h.key
	h.mu.Unlock()
	if readErr != nil {
		// the client broke the body off: heart acts on nothing
		return
	}
	if hook != nil && hook(w, r, rec) {
		return
	}
	if rec.Auth != "Bearer "+key {
		rest(w, http.StatusUnauthorized, "invalid api key")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v2/") {
		h.serveREST(w, r)
		return
	}
	tier := strings.TrimPrefix(r.URL.Path, "/mcp/")
	if r.Method == http.MethodDelete {
		h.mu.Lock()
		delete(h.sessions, rec.Session)
		h.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
		return
	}
	known := func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		return h.sessions[rec.Session]
	}
	if len(msg.ID) == 0 {
		if rec.Session != "" && tier != "full" && !known() {
			rest(w, http.StatusNotFound, "unknown or expired MCP session")
			return
		}
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if msg.Method == "initialize" && rec.Session == "" {
		version := contract.HeartVersions[len(contract.HeartVersions)-1]
		for _, v := range contract.HeartVersions {
			if v == msg.Params.ProtocolVersion {
				version = v
			}
		}
		if tier != "full" {
			h.mu.Lock()
			h.opened++
			id := fmt.Sprintf("session-%d", h.opened)
			h.sessions[id] = true
			h.mu.Unlock()
			w.Header().Set(contract.SessionHeader, id)
		}
		rpcResult(w, msg.ID, map[string]any{"protocolVersion": version, "capabilities": map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{"name": "anytype", "version": "1"}})
		return
	}
	if tier != "full" {
		if rec.Session == "" {
			rest(w, http.StatusBadRequest, "Mcp-Session-Id is required")
			return
		}
		if !known() {
			rest(w, http.StatusNotFound, "unknown or expired MCP session")
			return
		}
	}
	switch msg.Method {
	case "ping":
		rpcResult(w, msg.ID, map[string]any{})
	case "tools/list":
		h.mu.Lock()
		tools := append([]any{}, h.tools...)
		h.mu.Unlock()
		rpcResult(w, msg.ID, map[string]any{"tools": tools})
	case "tools/call":
		rpcResult(w, msg.ID, map[string]any{"content": []map[string]string{{"type": "text", "text": "done"}}})
	default:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": msg.ID, "error": map[string]any{"code": -32601, "message": "unknown method"}})
	}
}

func envelope(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "code": code, "message": msg})
}

// serveREST mimics the three /v2 routes the local file tools use.
func (h *fakeHeart) serveREST(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v2/spaces/"), "/")
	h.mu.Lock()
	files := h.files
	h.mu.Unlock()
	switch {
	case r.Method == http.MethodGet && len(parts) == 3 && parts[1] == "objects":
		f, ok := files[parts[2]]
		if !ok {
			envelope(w, http.StatusNotFound, "not_found", "object not found")
			return
		}
		stem, ext := f.name, ""
		if i := strings.LastIndex(f.name, "."); i > 0 {
			stem, ext = f.name[:i], f.name[i+1:]
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"object": "object", "id": parts[2],
			"properties": map[string]any{"name": stem, "file_ext": ext}})
	case r.Method == http.MethodGet && len(parts) == 4 && parts[1] == "files" && parts[3] == "content":
		f, ok := files[parts[2]]
		if !ok {
			envelope(w, http.StatusNotFound, "not_found", "file not found")
			return
		}
		w.Header().Set("Content-Type", f.mediaType)
		_, _ = w.Write(f.bytes)
	case r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "files":
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("dry_run") == "true" {
			_, _ = io.WriteString(w, `{"dry_run":true}`)
			return
		}
		h.mu.Lock()
		h.stored++
		h.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"object":"file","id":"uploaded-file-id"}`)
	default:
		envelope(w, http.StatusNotFound, "not_found", "no such route")
	}
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

type clock struct{ offset atomic.Int64 }

func (c *clock) now() time.Time          { return time.Now().Add(time.Duration(c.offset.Load())) }
func (c *clock) advance(d time.Duration) { c.offset.Add(int64(d)) }

// memEndpoint is an endpoint file in memory: a port, or a failure. Port 0
// reads as a missing file.
type memEndpoint struct {
	mu    sync.Mutex
	port  int
	err   error
	reads int
}

func (m *memEndpoint) Read(context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reads++
	switch {
	case m.err != nil:
		return 0, m.err
	case m.port == 0:
		return 0, &store.Error{Reason: store.ReasonMissing, Path: "endpoint.json"}
	}
	return m.port, nil
}

func (m *memEndpoint) set(port int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.port, m.err = port, nil
}

func (m *memEndpoint) fail(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func (m *memEndpoint) readCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reads
}

type harness struct {
	t     *testing.T
	ep    *memEndpoint
	heart *fakeHeart
	clock *clock
	in    *io.PipeWriter
	outR  *io.PipeReader
	lines chan string
	all   *syncBuffer // everything written to stdout
	log   *syncBuffer
	exit  chan int
	done  chan struct{} // closed when Serve has returned
	// root is the folder of the local file tools; empty with tempFiles,
	// where a full-tier Serve makes its own
	root      string
	tempFiles bool
	cancel    context.CancelFunc
	opts      Options
	srv       *server
}

type hopt func(*harness)

func withTier(tier string) hopt { return func(h *harness) { h.opts.Tier = tier } }

// withKey starts the forwarder with another key than the one heart takes.
func withKey(key string) hopt { return func(h *harness) { h.opts.Key = key } }

func withoutEndpoint() hopt { return func(h *harness) { h.ep.set(0) } }

// withFileEndpoint reads the port from a real endpoint file under a
// scratch home, written as the desktop app writes it, naming heart.
func withFileEndpoint(out **store.File) hopt {
	return func(h *harness) {
		home := h.t.TempDir()
		profileHome(h.t, home)
		f := store.OpenAt(home, store.NamespaceDev)
		writeEndpointFile(h.t, f.Path(), h.heart.port())
		h.opts.Endpoint = f
		*out = f
	}
}

func writeEndpointFile(t *testing.T, path string, port int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`{"v":1,"port":%d}`, port)), 0o600))
}

// withTempFiles leaves the folder of the file tools to Serve.
func withTempFiles() hopt { return func(h *harness) { h.tempFiles = true } }

func withOptions(f func(*Options)) hopt { return func(h *harness) { f(&h.opts) } }

// start runs Serve over pipes against a fake heart: a small tier, the
// key heart takes and an endpoint that names heart, unless an option says
// otherwise. At cleanup the key must be on neither stdout nor stderr.
func start(t *testing.T, opts ...hopt) *harness {
	h := &harness{t: t, ep: &memEndpoint{}, heart: newFakeHeart(t), clock: &clock{},
		all: &syncBuffer{}, log: &syncBuffer{}, exit: make(chan int, 1), done: make(chan struct{}), lines: make(chan string, 256)}
	h.ep.set(h.heart.port())
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	h.in, h.outR = inW, outR
	h.opts = Options{Key: canaryKey, Tier: "small", Endpoint: h.ep, In: inR, Out: outW, Log: h.log, Now: h.clock.now,
		ShutdownDeadline: 2 * time.Second, httpClient: newHTTPClient()}
	for _, o := range opts {
		o(h)
	}
	if h.opts.Files == nil && !h.tempFiles {
		// never the system's temporary folder by accident
		dir, err := files.Open(filepath.Join(t.TempDir(), "files"))
		require.NoError(t, err)
		h.opts.Files = dir
		// with the links in the test folder's own path resolved
		h.root = dir.Path()
	}
	h.opts.onServer = func(s *server) { h.srv = s }
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		sc := bufio.NewScanner(io.TeeReader(outR, h.all))
		sc.Buffer(make([]byte, 0, 1<<20), 256<<20)
		for sc.Scan() {
			h.lines <- sc.Text()
		}
		close(h.lines)
	}()
	go func() {
		code := Serve(ctx, h.opts)
		close(h.done)
		h.exit <- code
	}()
	t.Cleanup(func() {
		cancel()
		_ = inW.Close()
		_ = outR.Close()
		// the folder is released before the test's own folders go
		select {
		case <-h.done:
		case <-time.After(5 * time.Second):
		}
		// H1 case 16: never the key on stdout or stderr
		if h.opts.Key != "" {
			assert.NotContains(t, h.all.String(), h.opts.Key, "the key on stdout")
			assert.NotContains(t, h.log.String(), h.opts.Key, "the key on stderr")
		}
	})
	return h
}

// stopHeart closes the fake heart and the forwarder's idle connections to
// it, so the next request has to dial: without this the request may ride a
// connection that is closing, which is "lost after sending", not "refused".
func (h *harness) stopHeart() {
	h.heart.srv.Close()
	h.opts.httpClient.CloseIdleConnections()
}

func (h *harness) send(line string) {
	h.t.Helper()
	_, err := io.WriteString(h.in, line+"\n")
	require.NoError(h.t, err)
}

type got struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	} `json:"error"`
	raw string
}

func (g got) isError() bool {
	var r struct {
		IsError bool `json:"isError"`
	}
	_ = json.Unmarshal(g.Result, &r)
	return r.IsError
}

func (g got) text() string {
	var r struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	_ = json.Unmarshal(g.Result, &r)
	if len(r.Content) == 0 {
		return ""
	}
	return r.Content[0].Text
}

func (h *harness) recv() got {
	h.t.Helper()
	select {
	case line, ok := <-h.lines:
		require.True(h.t, ok, "stdout closed; log:\n%s", h.log.String())
		var g got
		require.NoError(h.t, json.Unmarshal([]byte(line), &g), "stdout line is not JSON: %q", line)
		g.raw = line
		return g
	case <-time.After(5 * time.Second):
		h.t.Fatalf("no reply; log:\n%s", h.log.String())
		return got{}
	}
}

// silent asserts nothing arrives on stdout for a moment.
func (h *harness) silent() {
	h.t.Helper()
	select {
	case line := <-h.lines:
		h.t.Fatalf("unexpected stdout line: %s", line)
	case <-time.After(150 * time.Millisecond):
	}
}

func (h *harness) call(line string) got {
	h.t.Helper()
	h.send(line)
	return h.recv()
}

// initialize runs the handshake and returns the client-visible version.
func (h *harness) initialize() string {
	h.t.Helper()
	g := h.call(`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
	require.Nil(h.t, g.Error, g.raw)
	var r struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	require.NoError(h.t, json.Unmarshal(g.Result, &r))
	h.send(`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	require.Eventually(h.t, func() bool { return h.heart.count("notifications/initialized") >= 1 }, 3*time.Second, 5*time.Millisecond)
	return r.ProtocolVersion
}

// warned waits until the reset warning's line is fully on stdout and the
// gate is open again. A client that answers the warning in the same
// instant may get it once more; tests wait instead.
func (h *harness) warned() {
	h.t.Helper()
	require.Eventually(h.t, func() bool {
		h.srv.mu.Lock()
		defer h.srv.mu.Unlock()
		return !h.srv.resetPending
	}, 3*time.Second, time.Millisecond)
}

func (h *harness) deletes(session string) int {
	n := 0
	for _, r := range h.heart.requests() {
		if r.Method == http.MethodDelete && (session == "" || r.Session == session) {
			n++
		}
	}
	return n
}

func (h *harness) waitExit() int {
	h.t.Helper()
	select {
	case code := <-h.exit:
		return code
	case <-time.After(10 * time.Second):
		h.t.Fatalf("Serve did not return; log:\n%s", h.log.String())
		return -1
	}
}

func toolCall(id any) string {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": map[string]any{"name": "x", "arguments": map[string]any{}}})
	return string(b)
}

// profileHome gives a scratch home a normal Windows profile's access list
// (the user, SYSTEM and Administrators, inherited by what is made in it),
// so no test depends on what the runner's temporary folder hands down.
// Elsewhere a scratch home is already a normal one.
func profileHome(t *testing.T, home string) {
	t.Helper()
	if runtime.GOOS != "windows" {
		return
	}
	me, err := user.Current()
	require.NoError(t, err)
	for _, args := range [][]string{
		{home, "/grant:r", "*" + me.Uid + ":(OI)(CI)F", "*S-1-5-18:(OI)(CI)F", "*S-1-5-32-544:(OI)(CI)F"},
		{home, "/inheritance:r"},
	} {
		out, err := exec.Command("icacls", args...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
}
