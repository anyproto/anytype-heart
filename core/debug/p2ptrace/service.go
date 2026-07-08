// Package p2ptrace is throwaway DEBUG-ONLY instrumentation.
//
// It captures the #p2p mDNS trace output (emitted by the local zeroconf fork via
// zeroconf.SetTraceWriter) into a size-capped rotating file, and serves that file
// over HTTP on :6061/trace. This lets a device be left running unattended (with
// the debugger detached) and the trace pulled later over the LAN:
//
//	curl http://<device-ip>:6061/trace   > p2p.log
//	curl http://<device-ip>:6061/trace/clear
//
// Remove this component (and its Register in bootstrap.go, plus the local
// go.mod replace of zeroconf) before merging.
package p2ptrace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/libp2p/zeroconf/v2"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const CName = "debug.p2ptrace"

const (
	listenAddr  = ":6061"
	maxFileSize = 64 << 20 // rotate at 64 MiB => <=128 MiB kept across the two files
)

var log = logging.Logger("p2ptrace")

func New() app.ComponentRunnable {
	return &service{}
}

type service struct {
	path     string
	addr     string
	file     *cappedWriter
	server   *http.Server
	stop     chan struct{}
	stopOnce sync.Once

	mu    sync.Mutex
	curLn net.Listener // active listener, replaced on each re-listen
}

func (s *service) Name() string { return CName }

func (s *service) Init(a *app.App) error {
	if s.addr == "" {
		s.addr = listenAddr
	}
	s.path = filepath.Join(os.TempDir(), "anytype_p2p_trace.log")
	cw, err := newCappedWriter(s.path, maxFileSize)
	if err != nil {
		// Non-fatal: without a file we simply leave zeroconf logging to stdout.
		log.Errorf("open trace file %s: %v", s.path, err)
		return nil
	}
	s.file = cw
	// Tee #p2p output to the device console AND the file.
	zeroconf.SetTraceWriter(io.MultiWriter(os.Stdout, cw))
	_, _ = fmt.Fprintf(cw, "#p2p ==== trace started %s, serving on %s/trace ====\n",
		time.Now().Format(time.RFC3339), listenAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/trace", s.handleTrace)
	mux.HandleFunc("/trace/clear", s.handleClear)
	mux.HandleFunc("/", s.handleIndex)
	s.server = &http.Server{Handler: mux}
	s.stop = make(chan struct{})
	return nil
}

func (s *service) Run(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	go s.serveLoop()
	return nil
}

// serveLoop keeps a listener alive across network changes. On iOS a Wi-Fi
// reconnect (often together with the app suspending in background) can invalidate
// the listening socket, making Serve return; we recreate the listener and carry
// on instead of dying until the next app restart. Only a real Close() stops it.
func (s *service) serveLoop() {
	const maxBackoff = 30 * time.Second
	backoff := time.Second
	for {
		select {
		case <-s.stop:
			return
		default:
		}

		ln, err := net.Listen("tcp", s.addr)
		if err != nil {
			log.Warnf("trace listen %s failed: %v (retry in %s)", s.addr, err, backoff)
			if !s.wait(backoff) {
				return
			}
			if backoff < maxBackoff {
				backoff *= 2
			}
			continue
		}
		s.mu.Lock()
		s.curLn = ln
		s.mu.Unlock()
		backoff = time.Second
		log.Warnf("serving #p2p trace on http://<device-ip>%s/trace (file %s)", s.addr, s.path)

		// Serve blocks until the listener dies or Shutdown() is called.
		err = s.server.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			return // Close() was called
		}
		log.Warnf("trace server on %s stopped: %v (re-listening)", s.addr, err)
		if !s.wait(time.Second) {
			return
		}
	}
}

// wait sleeps for d, returning false if a stop was requested meanwhile.
func (s *service) wait(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-s.stop:
		return false
	case <-t.C:
		return true
	}
}

func (s *service) Close(ctx context.Context) error {
	if s.stop != nil {
		s.stopOnce.Do(func() { close(s.stop) })
	}
	s.mu.Lock()
	ln := s.curLn
	s.mu.Unlock()
	if ln != nil {
		_ = ln.Close()
	}
	if s.server != nil {
		_ = s.server.Shutdown(ctx)
	}
	zeroconf.SetTraceWriter(nil)
	if s.file != nil {
		_ = s.file.Close()
	}
	return nil
}

func (s *service) handleTrace(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if s.file == nil {
		http.Error(w, "trace file not available", http.StatusServiceUnavailable)
		return
	}
	if err := s.file.writeTo(w); err != nil {
		log.Errorf("serve trace: %v", err)
	}
}

func (s *service) handleClear(w http.ResponseWriter, r *http.Request) {
	// Destructive: require POST so a stray GET (browser prefetch, link preview,
	// LAN port scan) can't wipe an in-progress capture.
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "use POST to clear the trace", http.StatusMethodNotAllowed)
		return
	}
	if s.file != nil {
		s.file.truncate()
	}
	_, _ = io.WriteString(w, "cleared\n")
}

func (s *service) handleIndex(w http.ResponseWriter, r *http.Request) {
	_, _ = fmt.Fprintf(w, "anytype #p2p trace\n\n"+
		"GET  /trace        dump the #p2p trace log\n"+
		"POST /trace/clear  truncate the log\n\n"+
		"file: %s (rotates at %d MiB)\n", s.path, maxFileSize>>20)
}

// cappedWriter is a thread-safe append writer that rotates to <path>.1 once the
// current file reaches max bytes, bounding disk use to ~2*max for long runs.
type cappedWriter struct {
	mu   sync.Mutex
	path string
	max  int64
	f    *os.File
	size int64
}

func newCappedWriter(path string, max int64) (*cappedWriter, error) {
	// Append, NOT truncate: survive app restarts so an intermittent spin captured
	// before a relaunch isn't lost. Growth stays bounded by the rotation cap.
	// Use /trace/clear to reset on demand.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	var size int64
	if fi, statErr := f.Stat(); statErr == nil {
		size = fi.Size()
	}
	return &cappedWriter{path: path, max: max, f: f, size: size}, nil
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f == nil {
		return 0, os.ErrClosed
	}
	if c.size+int64(len(p)) > c.max {
		c.rotateLocked()
		if c.f == nil { // reopen after rotation failed (e.g. disk full)
			return 0, os.ErrClosed
		}
	}
	n, err := c.f.Write(p)
	c.size += int64(n)
	return n, err
}

func (c *cappedWriter) rotateLocked() {
	_ = c.f.Close()
	_ = os.Rename(c.path, c.path+".1")
	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		c.f = nil
		return
	}
	c.f = f
	c.size = 0
}

// writeTo streams the rotated-out chunk (<path>.1) then the current file, so the
// response is chronological. It copies unlocked (only syncs under the lock) to
// avoid stalling trace writers during a slow HTTP transfer.
func (c *cappedWriter) writeTo(w io.Writer) error {
	c.mu.Lock()
	if c.f != nil {
		_ = c.f.Sync()
	}
	path := c.path
	c.mu.Unlock()

	if old, err := os.Open(path + ".1"); err == nil {
		_, _ = io.Copy(w, old)
		_ = old.Close()
	}
	cur, err := os.Open(path)
	if err != nil {
		return err
	}
	defer cur.Close()
	_, err = io.Copy(w, cur)
	return err
}

func (c *cappedWriter) truncate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = os.Remove(c.path + ".1")
	if c.f != nil {
		_ = c.f.Truncate(0)
		_, _ = c.f.Seek(0, 0)
		c.size = 0
	}
}

func (c *cappedWriter) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.f != nil {
		err := c.f.Close()
		c.f = nil
		return err
	}
	return nil
}
