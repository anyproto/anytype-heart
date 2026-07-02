package spacev2

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

// Backend performs the real per-space operations. All methods are invoked
// sequentially from the controller's reconcile goroutine (plus one final
// Unload from Close after the loop has exited), so implementations need no
// internal serialization against each other.
//
// Error contract: returned errors are retried by the controller with
// exponential backoff; wrap non-retryable failures with Fatal. Offload must be
// idempotent (a space may offload again in a later session, or having never
// been loaded).
type Backend interface {
	// AccountStatus returns the live persistent status from the SpaceView.
	AccountStatus(ctx context.Context) (spaceinfo.AccountStatus, error)
	// Load builds a usable space and runs the post-load pipeline. It owns the
	// LocalStatus publication (Loading -> Ok/Missing) and the OnSpaceLoad
	// listener call.
	Load(ctx context.Context) (clientspace.Space, error)
	// Unload releases the resident space keeping on-disk data intact (pause).
	// It must not write LocalStatusMissing; it owns the OnSpaceUnload call.
	Unload(ctx context.Context, sp clientspace.Space) error
	// Offload deletes local storage, files and indexes and writes
	// LocalStatusMissing. Called only when nothing is resident.
	Offload(ctx context.Context) error
	// Join runs the join waiter until the join resolves; it writes the
	// resulting AccountStatus (Active or Deleted) to the SpaceView itself and
	// returns nil only once that write happened.
	Join(ctx context.Context) error
}

const (
	defaultRetryMin = time.Second
	defaultRetryMax = 20 * time.Second
	retryFactor     = 1.5
)

type controllerOptions struct {
	retryMin time.Duration
	retryMax time.Duration
}

// controller reconciles one space toward the target derived from its live
// SpaceView status and the local demand flag. One goroutine (run) performs
// every mutating step, which is what serializes load/unload/offload/join
// against each other on the space's storage.
type controller struct {
	spaceId string
	backend Backend
	opts    controllerOptions

	mu      sync.Mutex
	state   State
	target  Target
	space   clientspace.Space
	wanted  bool
	lastErr error         // last failed step error; nil once a step succeeds
	changed chan struct{} // closed and replaced on every observable change
	// inputSeq advances on every poke; decidedSeq records which inputSeq the
	// current target/lastErr were derived from. Terminal answers (deleted,
	// fatal error) are only trusted when decidedSeq == inputSeq — a stale
	// decision must never fail a waiter whose input change is still unread.
	inputSeq   uint64
	decidedSeq uint64

	poke   chan struct{} // buffered 1; coalesces wakeups, single consumer
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

func newController(spaceId string, backend Backend, opts controllerOptions) *controller {
	if opts.retryMin <= 0 {
		opts.retryMin = defaultRetryMin
	}
	if opts.retryMax <= 0 {
		opts.retryMax = defaultRetryMax
	}
	c := &controller{
		spaceId: spaceId,
		backend: backend,
		opts:    opts,
		state:   StateIdle,
		target:  TargetIdle,
		changed: make(chan struct{}),
		poke:    make(chan struct{}, 1),
		done:    make(chan struct{}),
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	go c.run()
	return c
}

func (c *controller) SpaceId() string {
	return c.spaceId
}

// Poke wakes the reconcile loop to re-read inputs. Non-blocking; bursts
// coalesce. Because only the loop consumes the buffered slot, a poke sent
// after an input write is never lost.
func (c *controller) Poke() {
	c.mu.Lock()
	c.inputSeq++
	c.mu.Unlock()
	select {
	case c.poke <- struct{}{}:
	default:
	}
}

// SetWanted sets the demand flag: whether the space should be resident in
// memory. Deletion (AccountStatusDeleted) wins over demand.
func (c *controller) SetWanted(wanted bool) {
	c.mu.Lock()
	changed := c.wanted != wanted
	c.wanted = wanted
	c.mu.Unlock()
	if changed {
		c.Poke()
	}
}

func (c *controller) Wanted() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wanted
}

func (c *controller) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

// SpaceIfLoaded returns the resident space, or nil when not loaded.
func (c *controller) SpaceIfLoaded() clientspace.Space {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state == StateLoaded {
		return c.space
	}
	return nil
}

// WaitLoaded blocks until the space is loaded and returns it. It fails with
// ErrSpaceDeleted once the space converges toward offload, with the step
// error once the current attempt fails fatally, and with ErrClosed after
// Close. It does not add demand: callers that need the space resident must
// SetWanted(true) first.
func (c *controller) WaitLoaded(ctx context.Context) (clientspace.Space, error) {
	for {
		c.mu.Lock()
		fresh := c.decidedSeq == c.inputSeq
		switch {
		case c.state == StateLoaded:
			sp := c.space
			c.mu.Unlock()
			return sp, nil
		case c.state == StateClosed:
			c.mu.Unlock()
			return nil, ErrClosed
		case fresh && c.target == TargetOffloaded:
			c.mu.Unlock()
			return nil, ErrSpaceDeleted
		case fresh && c.lastErr != nil && isFatal(c.lastErr):
			err := c.lastErr
			c.mu.Unlock()
			return nil, err
		}
		ch := c.changed
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ch:
		}
	}
}

// Close stops the reconcile loop, releases a resident space using the
// caller's ctx, and fails in-flight waiters with ErrClosed. Safe to call
// multiple times.
func (c *controller) Close(ctx context.Context) error {
	c.cancel()
	<-c.done
	c.mu.Lock()
	sp := c.space
	state := c.state
	c.space = nil
	c.setStateLocked(StateClosed)
	c.mu.Unlock()
	if state == StateLoaded && sp != nil {
		if err := c.backend.Unload(ctx, sp); err != nil {
			return fmt.Errorf("unload space %s on close: %w", c.spaceId, err)
		}
	}
	return nil
}

// run is the reconcile loop: re-read live inputs, decide the target, perform
// one blocking step toward it, repeat. Exits only on Close.
func (c *controller) run() {
	defer close(c.done)
	retry := c.opts.retryMin
	for {
		if c.ctx.Err() != nil {
			return
		}
		// Capture the input generation AND the demand snapshot together,
		// BEFORE reading the status. The decision below must use exactly the
		// inputs that existed at seq: reading wanted later would let a
		// demand flip racing the status read produce a decision that acted
		// on newer input while stamped with the older seq — its fatal
		// outcome would then be treated as stale and silently re-attempted.
		// A status write racing the read is handled conservatively instead
		// (inputSeq > seq → the decision never parks or fails a waiter).
		c.mu.Lock()
		seq := c.inputSeq
		wanted := c.wanted
		c.mu.Unlock()

		status, err := c.backend.AccountStatus(c.ctx)
		if err != nil {
			if c.ctx.Err() != nil {
				return
			}
			c.setErr(fmt.Errorf("read account status of %s: %w", c.spaceId, err))
			if !c.sleep(retry) {
				return
			}
			retry = c.nextRetry(retry)
			continue
		}

		c.mu.Lock()
		target := decide(status, wanted)
		changed := target != c.target || c.decidedSeq != seq
		c.target = target
		c.decidedSeq = seq
		if changed {
			retry = c.opts.retryMin // input changed: reset backoff
			c.broadcastLocked()
		}
		state := c.state
		c.mu.Unlock()

		if converged(state, target) {
			c.clearErr()
			retry = c.opts.retryMin
			if !c.waitInputChange(seq) {
				return
			}
			continue
		}

		if err = c.step(state, target); err != nil {
			if c.ctx.Err() != nil {
				return
			}
			log.Warn("reconcile step failed",
				zap.String("spaceId", c.spaceId),
				zap.String("state", state.String()),
				zap.String("target", target.String()),
				zap.Error(err))
			c.setErr(err)
			if isFatal(err) {
				// Park until an input actually changes; then clear the error
				// and re-attempt.
				if !c.waitInputChange(seq) {
					return
				}
				c.clearErr()
				continue
			}
			if !c.sleep(retry) {
				return
			}
			retry = c.nextRetry(retry)
			continue
		}
		c.clearErr()
		retry = c.opts.retryMin
	}
}

// step performs exactly one convergence edge; see DESIGN.md §2.
func (c *controller) step(state State, target Target) error {
	switch {
	case state == StateLoaded:
		// Any non-Loaded target first releases the resident space; the next
		// iteration continues (e.g. toward offload).
		return c.unloadStep()
	case target == TargetLoaded:
		return c.loadStep()
	case target == TargetJoining:
		return c.joinStep()
	case target == TargetOffloaded:
		return c.offloadStep()
	}
	return nil
}

func (c *controller) loadStep() error {
	c.setState(StateLoading)
	sp, err := c.backend.Load(c.ctx)
	if err != nil {
		c.setState(StateIdle)
		return fmt.Errorf("load space %s: %w", c.spaceId, err)
	}
	c.mu.Lock()
	c.space = sp
	c.setStateLocked(StateLoaded)
	c.mu.Unlock()
	return nil
}

func (c *controller) unloadStep() error {
	c.mu.Lock()
	sp := c.space
	c.space = nil
	c.setStateLocked(StateUnloading)
	c.mu.Unlock()
	err := c.backend.Unload(c.ctx, sp)
	c.setState(StateIdle)
	if err != nil {
		// Residency is dropped regardless: a half-closed space must not be
		// handed to waiters or unloaded twice.
		return fmt.Errorf("unload space %s: %w", c.spaceId, err)
	}
	return nil
}

func (c *controller) offloadStep() error {
	c.setState(StateOffloading)
	if err := c.backend.Offload(c.ctx); err != nil {
		c.setState(StateIdle)
		return fmt.Errorf("offload space %s: %w", c.spaceId, err)
	}
	c.setState(StateOffloaded)
	return nil
}

func (c *controller) joinStep() error {
	c.setState(StateJoining)
	err := c.backend.Join(c.ctx)
	c.setState(StateIdle)
	if err != nil {
		return fmt.Errorf("join space %s: %w", c.spaceId, err)
	}
	return nil
}

func (c *controller) setState(s State) {
	c.mu.Lock()
	c.setStateLocked(s)
	c.mu.Unlock()
}

func (c *controller) setStateLocked(s State) {
	if c.state == s {
		return
	}
	c.state = s
	c.broadcastLocked()
}

func (c *controller) setErr(err error) {
	c.mu.Lock()
	c.lastErr = err
	c.broadcastLocked()
	c.mu.Unlock()
}

func (c *controller) clearErr() {
	c.mu.Lock()
	if c.lastErr != nil {
		c.lastErr = nil
		c.broadcastLocked()
	}
	c.mu.Unlock()
}

func (c *controller) broadcastLocked() {
	close(c.changed)
	c.changed = make(chan struct{})
}

// waitInputChange blocks until some input newer than decidedSeq arrives;
// returns false when closing. A poke token left over from an input change
// this decision already incorporated does not wake the park.
func (c *controller) waitInputChange(decidedSeq uint64) bool {
	for {
		c.mu.Lock()
		fresh := c.inputSeq > decidedSeq
		c.mu.Unlock()
		if fresh {
			return true
		}
		select {
		case <-c.ctx.Done():
			return false
		case <-c.poke:
		}
	}
}

// sleep waits for d, interruptible by a poke; returns false when closing.
func (c *controller) sleep(d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-c.ctx.Done():
		return false
	case <-c.poke:
		return true
	case <-t.C:
		return true
	}
}

func (c *controller) nextRetry(cur time.Duration) time.Duration {
	next := time.Duration(float64(cur) * retryFactor)
	if next > c.opts.retryMax {
		next = c.opts.retryMax
	}
	return next
}
