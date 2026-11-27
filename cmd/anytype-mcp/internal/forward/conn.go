package forward

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/anyproto/anytype-heart/cmd/anytype-mcp/internal/store"
)

// snapshot is what one exchange runs on: the item as last read.
type snapshot struct {
	key  string
	port int
	tier string
}

// connection is the item as this process sees it. The account and the
// tier are pinned by the one read at start; a process that started without
// a readable item has no owner to pin and stays not connected for its
// whole life. Later reads only move the port and the key.
type connection struct {
	id       string
	st       store.Store
	now      func() time.Time
	maxAge   time.Duration
	portOver int
	tierOver string
	log      *logger

	gone    atomic.Bool // set with dead
	mu      sync.Mutex
	dead    string // why not connected; empty while connected
	account string
	tier    string
	snap    snapshot
	readAt  time.Time
}

// errNotConnected is every request's answer once the connection is dead.
var errNotConnected = errors.New("not connected")

func (c *connection) read(ctx context.Context) (store.Item, error) {
	raw, err := c.st.Read(ctx, c.id, false)
	if err != nil {
		return store.Item{}, err
	}
	it, err := store.DecodeItem(raw)
	if err != nil {
		return store.Item{}, err
	}
	if c.portOver != 0 {
		it.Port = c.portOver
	}
	if c.tierOver != "" {
		it.Tier = c.tierOver
	}
	return it, nil
}

func describe(err error) string {
	var t *store.TemporaryError
	switch {
	case errors.Is(err, store.ErrNotFound):
		return "no item"
	case errors.Is(err, store.ErrNoStore):
		return "no key store"
	case errors.Is(err, store.ErrInvalidItem):
		return "invalid item"
	case errors.As(err, &t):
		return "key store " + t.Reason
	}
	return "key store error"
}

// start makes the one read that decides this process's fate.
func (c *connection) start(ctx context.Context) {
	it, err := c.read(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.kill(describe(err), "not connected: %s; restart the AI app after connecting in Anytype")
		return
	}
	c.account, c.tier = it.Account, it.Tier
	c.snap = snapshot{key: it.Key, port: it.Port, tier: it.Tier}
	c.readAt = c.now()
	c.log.printf("connected: tier %s, item rev %d", it.Tier, it.Rev)
}

// connected is read by the stdin reader, which must not wait behind a
// slow store read: it looks at a flag, not at the lock.
func (c *connection) connected() bool { return !c.gone.Load() }

// kill ends the connection for the life of the process. The caller holds
// the lock.
func (c *connection) kill(reason, logged string) {
	c.dead = reason
	c.gone.Store(true)
	c.log.printf(logged, reason)
}

// current returns the snapshot for a request, re-reading the item first
// when the last read is older than the refresh window.
func (c *connection) current(ctx context.Context) (snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead != "" {
		return snapshot{}, errNotConnected
	}
	if c.now().Sub(c.readAt) >= c.maxAge {
		c.refreshLocked(ctx)
	}
	if c.dead != "" {
		return snapshot{}, errNotConnected
	}
	return c.snap, nil
}

// refresh re-reads the item now: after a refusal and after a 401.
func (c *connection) refresh(ctx context.Context) (snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dead != "" {
		return snapshot{}, errNotConnected
	}
	c.refreshLocked(ctx)
	if c.dead != "" {
		return snapshot{}, errNotConnected
	}
	return c.snap, nil
}

// refreshLocked compares the whole new snapshot. Another account, another
// tier or no item ends the connection until restart. A store that can't
// answer right now proves nothing: the last snapshot stays, and heart
// still refuses a revoked key.
func (c *connection) refreshLocked(ctx context.Context) {
	it, err := c.read(ctx)
	c.readAt = c.now()
	switch {
	case err == nil:
	case store.IsTemporary(err):
		c.log.printf("item re-read failed (%s); keeping the last snapshot", describe(err))
		return
	default:
		c.kill(describe(err), "not connected until restart: %s")
		return
	}
	if it.Account != c.account {
		c.kill("the connection now belongs to another account", "not connected until restart: %s")
		return
	}
	if it.Tier != c.tier {
		c.kill("the connection's tier changed", "not connected until restart: %s")
		return
	}
	next := snapshot{key: it.Key, port: it.Port, tier: it.Tier}
	if next != c.snap {
		c.log.printf("item changed: rev %d, port %d", it.Rev, it.Port)
	}
	c.snap = next
}
