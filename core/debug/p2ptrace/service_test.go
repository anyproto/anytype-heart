package p2ptrace

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCappedWriterRotationAndServe(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.log")
	// tiny cap so a couple of writes force a rotation
	cw, err := newCappedWriter(path, 32)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cw.Close() })

	_, _ = io.WriteString(cw, "AAAAAAAAAAAAAAAAAAAAAAAA\n") // 25 bytes, fits
	_, _ = io.WriteString(cw, "BBBBBBBBBBBBBBBBBBBBBBBB\n") // 25 bytes -> rotates first chunk to .log.1
	_, _ = io.WriteString(cw, "CCCC\n")

	s := &service{path: path, file: cw}
	rr := httptest.NewRecorder()
	s.handleTrace(rr, httptest.NewRequest("GET", "/trace", nil))

	body := rr.Body.String()
	require.Equal(t, 200, rr.Code)
	// chronological: rotated-out A's first, then current B's + C's
	require.True(t, strings.Contains(body, "AAAA"), "rotated chunk missing: %q", body)
	require.True(t, strings.Contains(body, "BBBB"), "current chunk missing: %q", body)
	require.True(t, strings.Contains(body, "CCCC"), "latest line missing: %q", body)
	require.Less(t, strings.Index(body, "AAAA"), strings.Index(body, "BBBB"), "not chronological")

	// a GET must NOT clear (destructive endpoint is POST-only)
	rrGet := httptest.NewRecorder()
	s.handleClear(rrGet, httptest.NewRequest("GET", "/trace/clear", nil))
	require.Equal(t, http.StatusMethodNotAllowed, rrGet.Code)
	rrStill := httptest.NewRecorder()
	s.handleTrace(rrStill, httptest.NewRequest("GET", "/trace", nil))
	require.NotEmpty(t, rrStill.Body.String(), "GET /trace/clear must not have wiped the log")

	// POST clears both files
	s.handleClear(httptest.NewRecorder(), httptest.NewRequest("POST", "/trace/clear", nil))
	rr2 := httptest.NewRecorder()
	s.handleTrace(rr2, httptest.NewRequest("GET", "/trace", nil))
	require.Empty(t, rr2.Body.String())
}

func TestServerAutoRestartsAfterListenerDies(t *testing.T) {
	// Fixed loopback port so the re-listen rebinds the same address the client uses.
	port := freePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	dir := t.TempDir()
	cw, err := newCappedWriter(filepath.Join(dir, "trace.log"), 1<<20)
	require.NoError(t, err)
	_, _ = io.WriteString(cw, "hello\n")

	mux := http.NewServeMux()
	s := &service{addr: addr, path: "test", file: cw, stop: make(chan struct{}), server: &http.Server{Handler: mux}}
	mux.HandleFunc("/trace", s.handleTrace)
	go s.serveLoop()
	t.Cleanup(func() { _ = s.Close(t.Context()) })

	require.Eventually(t, func() bool { return get(addr) == "hello\n" }, 3*time.Second, 20*time.Millisecond,
		"server never came up")

	// Simulate a network-induced listener death.
	s.mu.Lock()
	ln := s.curLn
	s.mu.Unlock()
	require.NoError(t, ln.Close())

	// It must re-listen on the same address and serve again.
	require.Eventually(t, func() bool { return get(addr) == "hello\n" }, 5*time.Second, 50*time.Millisecond,
		"server did not auto-restart after listener died")
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	p := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return p
}

func get(addr string) string {
	resp, err := http.Get("http://" + addr + "/trace")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

func TestSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.log")

	cw1, err := newCappedWriter(path, 1<<20)
	require.NoError(t, err)
	_, _ = io.WriteString(cw1, "session-1 line\n")
	require.NoError(t, cw1.Close())

	// simulate app restart: reopen the same path
	cw2, err := newCappedWriter(path, 1<<20)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cw2.Close() })
	_, _ = io.WriteString(cw2, "session-2 line\n")

	s := &service{path: path, file: cw2}
	rr := httptest.NewRecorder()
	s.handleTrace(rr, httptest.NewRequest("GET", "/trace", nil))
	body := rr.Body.String()
	require.Contains(t, body, "session-1 line", "pre-restart data was lost")
	require.Contains(t, body, "session-2 line")
}
