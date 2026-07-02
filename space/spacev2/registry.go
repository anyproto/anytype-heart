package spacev2

import (
	"context"
	"sync"
)

// builderFunc builds and starts a controller. Invoked only from the watcher
// apply path (unidirectional single-builder invariant).
type builderFunc func(ctx context.Context) (SpaceController, error)

type entryState int

const (
	// statePlaceholder: created by a waiter (await) before any build was
	// attempted. get() treats it as not-exists (v1 Get parity); await blocks.
	statePlaceholder entryState = iota
	stateBuilding
	stateReady
	stateFailed
)

type entry struct {
	state entryState
	ctrl  SpaceController
	err   error
	// ready is closed when the CURRENT attempt completes; a retry after a
	// failure swaps in a fresh channel, so waiters loop and re-read state.
	ready chan struct{}
	// work serializes the watcher apply (build + Update) per space (§9.2):
	// events for different spaces run concurrently, per-space strictly ordered.
	work sync.Mutex
}

// registry replaces v1's spaceControllers + waiting maps with one entry type
// carrying a retryable ready-future. Builds happen through ensure only (the
// watcher), so the v1 dual-path dedup (§9.9) and the session-sticky failed
// build (§9.8) cannot occur by construction.
type registry struct {
	mu      sync.Mutex
	entries map[string]*entry
	closing bool
}

func newRegistry() *registry {
	return &registry{entries: map[string]*entry{}}
}

// entryFor returns the entry, placeholder-creating it. Callers hold r.mu.
func (r *registry) entryFor(spaceId string) *entry {
	if e, ok := r.entries[spaceId]; ok {
		return e
	}
	e := &entry{state: statePlaceholder, ready: make(chan struct{})}
	r.entries[spaceId] = e
	return e
}

func (r *registry) workLock(spaceId string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &r.entryFor(spaceId).work
}

// await blocks until some attempt for spaceId completes. It resolves with the
// result of the attempt that completes while waiting; if the latest completed
// attempt failed and no retry is in flight, it returns that failure.
func (r *registry) await(ctx context.Context, spaceId string) (SpaceController, error) {
	for {
		r.mu.Lock()
		if r.closing {
			r.mu.Unlock()
			return nil, ErrSpaceIsClosing
		}
		e := r.entryFor(spaceId)
		state, ctrl, err, ready := e.state, e.ctrl, e.err, e.ready
		r.mu.Unlock()
		switch state {
		case stateReady:
			return ctrl, nil
		case stateFailed:
			return nil, err
		}
		select {
		case <-ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// get is the non-blocking-for-unknown-ids read (v1 Get parity): a space no
// build was ever attempted for reads as ErrSpaceNotExists even if waiters
// exist; an in-flight build is awaited; ready/failed resolve immediately.
func (r *registry) get(ctx context.Context, spaceId string) (SpaceController, error) {
	r.mu.Lock()
	e, ok := r.entries[spaceId]
	if !ok || e.state == statePlaceholder {
		r.mu.Unlock()
		return nil, ErrSpaceNotExists
	}
	r.mu.Unlock()
	return r.await(ctx, spaceId)
}

// ensure runs one build attempt for spaceId unless a controller is already
// ready. A previously failed entry is retried with a fresh ready channel —
// failures are never sticky for the session (fixes v1 §9.8).
func (r *registry) ensure(ctx context.Context, spaceId string, build builderFunc) (SpaceController, error) {
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return nil, ErrSpaceIsClosing
	}
	e := r.entryFor(spaceId)
	switch e.state {
	case stateReady:
		ctrl := e.ctrl
		r.mu.Unlock()
		return ctrl, nil
	case stateBuilding:
		// The per-space work lock serializes watcher applies, so a concurrent
		// build for the same space cannot happen; treat defensively as await.
		r.mu.Unlock()
		return r.await(ctx, spaceId)
	case stateFailed:
		e.ready = make(chan struct{})
		e.err = nil
	}
	e.state = stateBuilding
	ready := e.ready
	r.mu.Unlock()

	ctrl, err := build(ctx)

	r.mu.Lock()
	if err == nil && r.closing {
		// closeAll ran while this build was in flight; it skipped this entry
		// (only ensure may close the attempt channel), so the fresh controller
		// must be closed here instead of being published past the shutdown.
		err = ErrSpaceIsClosing
		r.mu.Unlock()
		if closeErr := ctrl.Close(ctx); closeErr != nil {
			log.Error("close controller built during shutdown", zapSpaceId(spaceId), zapError(closeErr))
		}
		r.mu.Lock()
		ctrl = nil
	}
	if err != nil {
		e.state = stateFailed
		e.err = err
	} else {
		e.state = stateReady
		e.ctrl = ctrl
	}
	close(ready)
	r.mu.Unlock()
	return ctrl, err
}

// addStatic registers an already-started controller (marketplace). Safe over a
// placeholder (resolves its waiters) or a failed/ready entry (their channel was
// already closed by the completing ensure); must not race an in-flight build —
// that channel belongs to its ensure.
func (r *registry) addStatic(spaceId string, ctrl SpaceController) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entryFor(spaceId)
	prev := e.state
	e.state = stateReady
	e.ctrl = ctrl
	e.err = nil
	if prev == statePlaceholder {
		close(e.ready)
	}
}

func (r *registry) all() []SpaceController {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrls := make([]SpaceController, 0, len(r.entries))
	for _, e := range r.entries {
		if e.state == stateReady {
			ctrls = append(ctrls, e.ctrl)
		}
	}
	return ctrls
}

func (r *registry) allIds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.entries))
	for id, e := range r.entries {
		if e.state == stateReady {
			ids = append(ids, id)
		}
	}
	return ids
}

// closeAll marks the registry closing (later ensure/await refuse with
// ErrSpaceIsClosing), releases blocked waiters, and closes all ready
// controllers concurrently.
func (r *registry) closeAll(ctx context.Context) error {
	r.mu.Lock()
	r.closing = true
	ctrls := make([]SpaceController, 0, len(r.entries))
	for _, e := range r.entries {
		if e.state == stateReady {
			ctrls = append(ctrls, e.ctrl)
			continue
		}
		// Only placeholder entries are resolved here: a building entry's
		// channel belongs to its in-flight ensure (which re-checks closing on
		// completion), and closing it twice would panic. Waiters on a building
		// entry wake when that build completes and observe closing on re-loop.
		if e.state == statePlaceholder {
			e.state = stateFailed
			e.err = ErrSpaceIsClosing
			close(e.ready)
		}
	}
	r.mu.Unlock()

	var wg sync.WaitGroup
	for _, ctrl := range ctrls {
		wg.Add(1)
		go func(c SpaceController) {
			defer wg.Done()
			if err := c.Close(ctx); err != nil {
				log.Error("close space controller", zapSpaceId(c.SpaceId()), zapError(err))
			}
		}(ctrl)
	}
	wg.Wait()
	return nil
}
