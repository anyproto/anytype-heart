package forward

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

// Endpoint reads the port of the local Anytype: the endpoint file
// (store.File) outside tests.
type Endpoint interface {
	Read(ctx context.Context) (int, error)
}

// snapshot is what one exchange runs on: the key and the tier this process
// was started with, and the port of a read that passed the gate.
type snapshot struct {
	key  string
	port int
	tier string
}

// MaxKeyBytes bounds the key.
const MaxKeyBytes = 1024

// keyProblem is why a key from the environment can't be used, or empty.
// The text never carries the key.
func keyProblem(key string) string {
	switch {
	case key == "" || key == "${user_config.api_key}":
		return "the app passed no key"
	case strings.HasPrefix(key, "__encrypted__:"):
		return "the app passed an encrypted value instead of the key"
	case len(key) > MaxKeyBytes:
		return "the app passed a key longer than 1024 characters"
	case strings.IndexFunc(key, func(r rune) bool { return r <= 0x20 || r >= 0x7f }) >= 0:
		// the key travels in an HTTP header: visible ASCII only
		return "the app passed a key with characters a header can't carry"
	}
	return ""
}

// connection is the key and the tier, fixed at start, and the endpoint as
// last read. Only a bad key ends it, for the life of the process; an
// endpoint that can't be read only stops the requests that find it so.
type connection struct {
	key  string
	tier string
	// dead is why this process never connects (a bad key); empty otherwise.
	// It is set once, before any goroutine starts.
	dead string

	ep     Endpoint
	now    func() time.Time
	maxAge time.Duration
	// timeout bounds one read; pending is a read that has not come back yet
	timeout time.Duration
	pending chan readResult
	log     *logger

	mu     sync.Mutex
	port   int       // of the last read; 0 when it failed
	readAt time.Time // the last attempt
	okAt   time.Time // the last read that succeeded
	failed string    // the last failure, as logged
}

var (
	// errNotConnected: the key is bad; final until restart.
	errNotConnected = errors.New("not connected")
	// errUnreachable: the endpoint can't be read safely right now; nothing
	// was sent.
	errUnreachable = errors.New("the endpoint can't be read")
	// errMoved: the endpoint now names another port than the one a
	// transfer or a session was pinned to; nothing was sent.
	errMoved = errors.New("the endpoint moved to another port")
)

func newConnection(o Options, log *logger) *connection {
	c := &connection{key: o.Key, tier: o.Tier, ep: o.Endpoint, now: o.Now, maxAge: o.RefreshAfter, timeout: o.ReadTimeout, log: log}
	if c.dead = keyProblem(o.Key); c.dead != "" {
		log.printf("not connected until restart: %s", c.dead)
	} else {
		log.printf("tier %s", c.tier)
	}
	return c
}

// connected is false for a bad key, for the life of the process.
func (c *connection) connected() bool { return c.dead == "" }

type readResult struct {
	port int
	err  error
}

// read reads the endpoint off the control path. The read runs in its own
// goroutine and is waited for at most the read timeout: a read that
// doesn't return in time (a stalled network home) is "can't be read right
// now" for this attempt. Its goroutine is abandoned, never awaited: while
// it is still out, later attempts fail at once, so a stuck file system
// can't hold a request, an EOF or a signal. What such a read returns
// later says nothing about now and is dropped: the next attempt reads
// afresh. A result counts from the moment its read started, and one that
// is only picked up after the timeout (a process that was suspended) is
// dropped too. The caller holds the lock.
func (c *connection) read(ctx context.Context, start time.Time) (int, error) {
	late := &store.Error{Reason: store.ReasonUnreachable, Rule: "the endpoint file did not answer in time"}
	if c.pending != nil {
		select {
		case <-c.pending:
			c.pending = nil
		default:
			return 0, late
		}
	}
	ch := make(chan readResult, 1)
	c.pending = ch
	go func() { //nolint:gosec // the read outlives the request that started it, by design
		port, err := c.ep.Read(context.Background())
		ch <- readResult{port, err}
	}()
	timer := time.NewTimer(c.timeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		c.pending = nil
		if c.now().Sub(start) >= c.timeout {
			return 0, late
		}
		return r.port, r.err
	case <-timer.C:
		return 0, late
	case <-ctx.Done():
		return 0, late
	}
}

// gate is called immediately before every request that carries the
// bearer. It passes only on a read that succeeded within the refresh
// window, reading again first when the last good one is older; after a
// failed read it reads at most once a second. A failure is never final and
// never falls back to an earlier port.
func (c *connection) gate(ctx context.Context) (snapshot, error) {
	if c.dead != "" {
		return snapshot{}, errNotConnected
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.port != 0 && c.now().Sub(c.okAt) < c.maxAge:
		return c.snapLocked(), nil
	case c.backingOff():
		return snapshot{}, errUnreachable
	}
	return c.readLocked(ctx)
}

// gateAt is the gate for a caller that pinned a port (each request of a
// transfer, a session's follow-ups): it reads the endpoint now, so a
// withdrawal between two requests stops the second, and it fails when the
// endpoint names another port.
func (c *connection) gateAt(ctx context.Context, pinned snapshot) error {
	cur, err := c.refresh(ctx)
	if err != nil {
		return err
	}
	if cur.port != pinned.port {
		return errMoved
	}
	return nil
}

// refresh reads the endpoint now (after a refused connection, which sent
// nothing, so the next request goes where Anytype is now), but not again
// within a second of a failed read.
func (c *connection) refresh(ctx context.Context) (snapshot, error) {
	if c.dead != "" {
		return snapshot{}, errNotConnected
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.backingOff() {
		return snapshot{}, errUnreachable
	}
	return c.readLocked(ctx)
}

// backingOff: the last read failed less than a second ago. The caller
// holds the lock.
func (c *connection) backingOff() bool {
	return c.port == 0 && !c.readAt.IsZero() && c.now().Sub(c.readAt) < time.Second
}

func (c *connection) readLocked(ctx context.Context) (snapshot, error) {
	start := c.now()
	port, err := c.read(ctx, start)
	c.readAt = start
	if err != nil {
		c.port = 0
		if text := store.AsError(err).Error(); text != c.failed {
			c.failed = text
			c.log.printf("Anytype can't be reached: %s", text)
		}
		return snapshot{}, errUnreachable
	}
	if port != c.port {
		c.log.printf("endpoint: port %d", port)
	}
	c.port, c.okAt, c.failed = port, c.readAt, ""
	return c.snapLocked(), nil
}

func (c *connection) snapLocked() snapshot {
	return snapshot{key: c.key, port: c.port, tier: c.tier}
}
