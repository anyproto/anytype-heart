package forward

import (
	"fmt"
	"io"
	"strings"
	"sync"
)

// logger writes one line per event to stderr. Callers pass only fixed
// texts, numbers, tiers and status codes: never the key, a header, a body
// or the environment.
type logger struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *logger) printf(format string, args ...any) {
	if l == nil || l.w == nil {
		return
	}
	line := strings.ReplaceAll(fmt.Sprintf(format, args...), "\n", " ")
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintf(l.w, "anytype-mcp: %s\n", line)
}
