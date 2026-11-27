package forward

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"
)

const (
	logQueue   = 64
	logLineMax = 512
)

// logger writes one line per event to stderr and never blocks its caller:
// lines wait in a small queue drained by one goroutine, and a line that
// finds the queue full is dropped and counted. A host that stops reading
// stderr can therefore never hold a worker or the shutdown.
//
// Callers pass fixed texts, numbers, tiers and status codes. A method name
// is the client's choice, so every control character is replaced and a
// line is capped. Never the key, a header, a body or the environment.
type logger struct {
	ch      chan string
	done    chan struct{}
	dropped atomic.Int64

	mu     sync.RWMutex
	closed bool
}

func newLogger(w io.Writer) *logger {
	l := &logger{ch: make(chan string, logQueue), done: make(chan struct{})}
	go func() {
		defer close(l.done)
		if w == nil {
			w = io.Discard
		}
		for line := range l.ch {
			if _, err := io.WriteString(w, line); err != nil {
				l.dropped.Add(1)
			}
		}
		if n := l.dropped.Load(); n > 0 {
			fmt.Fprintf(w, "anytype-mcp: %d log lines were dropped\n", n)
		}
	}()
	return l
}

func cleanLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	if len(s) > logLineMax {
		s = strings.ToValidUTF8(s[:logLineMax], "")
	}
	return s
}

func (l *logger) printf(format string, args ...any) {
	if l == nil {
		return
	}
	line := "anytype-mcp: " + cleanLine(fmt.Sprintf(format, args...)) + "\n"
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.closed {
		return
	}
	select {
	case l.ch <- line:
	default:
		l.dropped.Add(1)
	}
}

// close stops taking lines and gives the drain what is left of ctx.
func (l *logger) close(ctx context.Context) {
	l.mu.Lock()
	if !l.closed {
		l.closed = true
		close(l.ch)
	}
	l.mu.Unlock()
	select {
	case <-l.done:
	case <-ctx.Done():
	}
}
