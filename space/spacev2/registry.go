package spacev2

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// controllerFactory builds the controller for a space id. It must be cheap
// and non-blocking (no I/O): it runs under the registry lock, which is what
// guarantees a single controller per space with no separate dedup machinery.
type controllerFactory func(spaceId string) (*controller, error)

// registry tracks one controller per space. It never caches errors: a failed
// factory call leaves no entry behind, so the next caller simply retries.
type registry struct {
	mu     sync.Mutex
	ctrls  map[string]*controller
	newCtl controllerFactory
	closed bool
}

func newRegistry(factory controllerFactory) *registry {
	return &registry{
		ctrls:  make(map[string]*controller),
		newCtl: factory,
	}
}

// getOrCreate returns the controller for spaceId, creating it if absent.
// Idempotent; watcher events and direct API calls converge on one instance.
func (r *registry) getOrCreate(spaceId string) (*controller, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil, ErrClosed
	}
	if c, ok := r.ctrls[spaceId]; ok {
		return c, nil
	}
	c, err := r.newCtl(spaceId)
	if err != nil {
		return nil, fmt.Errorf("create controller for space %s: %w", spaceId, err)
	}
	r.ctrls[spaceId] = c
	return c, nil
}

// get returns the existing controller or nil.
func (r *registry) get(spaceId string) *controller {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.ctrls[spaceId]
}

// all returns a snapshot of the tracked controllers.
func (r *registry) all() []*controller {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrls := make([]*controller, 0, len(r.ctrls))
	for _, c := range r.ctrls {
		ctrls = append(ctrls, c)
	}
	return ctrls
}

// close refuses new controllers and closes the existing ones concurrently.
// Safe to call multiple times.
func (r *registry) close(ctx context.Context) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	ctrls := make([]*controller, 0, len(r.ctrls))
	for _, c := range r.ctrls {
		ctrls = append(ctrls, c)
	}
	r.mu.Unlock()

	errs := make([]error, len(ctrls))
	var wg sync.WaitGroup
	for i, c := range ctrls {
		wg.Add(1)
		go func(i int, c *controller) {
			defer wg.Done()
			errs[i] = c.Close(ctx)
		}(i, c)
	}
	wg.Wait()
	return errors.Join(errs...)
}
