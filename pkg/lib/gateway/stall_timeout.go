package gateway

import (
	"io"
	"sync"
	"time"
)

// stallTimeout cuts loose a transfer that has stopped making progress.
//
// It replaces a total deadline on the request: a large file on a slow link, or
// one heart is still fetching from a node, legitimately takes longer than any
// fixed budget, and a total deadline kills it mid-stream. What is worth killing
// is a transfer that has gone quiet — so the clock restarts on every byte, and
// only a window with no progress at all fires onStall.
type stallTimeout struct {
	mu      sync.Mutex
	timer   *time.Timer
	window  time.Duration
	stopped bool
}

// newStallTimeout arms the watchdog. onStall runs once the window passes with
// no progress reported; it must not block.
func newStallTimeout(window time.Duration, onStall func()) *stallTimeout {
	return &stallTimeout{
		timer:  time.AfterFunc(window, onStall),
		window: window,
	}
}

// progress restarts the window. It is called on every read that moved bytes.
func (s *stallTimeout) progress() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.stopped {
		return
	}
	s.timer.Reset(s.window)
}

// stop disarms the watchdog for good. A progress call after it is a no-op, so
// a reader still draining after the handler returned cannot re-arm it.
func (s *stallTimeout) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopped = true
	s.timer.Stop()
}

// progressReader reports every byte moved to a stallTimeout, which is what
// keeps a slow-but-alive transfer from being cut off.
type progressReader struct {
	io.ReadSeeker
	onProgress func()
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.ReadSeeker.Read(p)
	if n > 0 {
		r.onProgress()
	}
	return n, err
}

// Seek counts as progress too: a seek that returned did real work, and
// http.ServeContent seeks to size the content before it reads anything.
func (r *progressReader) Seek(offset int64, whence int) (int64, error) {
	pos, err := r.ReadSeeker.Seek(offset, whence)
	if err == nil {
		r.onProgress()
	}
	return pos, err
}
