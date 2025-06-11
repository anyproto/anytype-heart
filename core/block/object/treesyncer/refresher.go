package treesyncer

import (
	"context"
	"sync"
)

type refresher[T any] struct {
	action      func(ctx context.Context) T
	mu          sync.RWMutex
	onRefreshes []func(T)
	running     bool
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	closed      bool
}

func newRefresher[T any](action func(ctx context.Context) T) *refresher[T] {
	ctx, cancel := context.WithCancel(context.Background())
	return &refresher[T]{
		action: action,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (c *refresher[T]) doAfter(onRefresh func(T)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.onRefreshes = append(c.onRefreshes, onRefresh)
	if c.running {
		return
	}
	c.running = true
	c.wg.Add(1)

	go func() {
		defer c.wg.Done()
		result := c.action(c.ctx)
		c.mu.Lock()
		callbacks := make([]func(T), len(c.onRefreshes))
		copy(callbacks, c.onRefreshes)
		c.onRefreshes = nil
		closed := c.closed
		c.running = false
		c.mu.Unlock()

		if !closed {
			for _, callback := range callbacks {
				callback(result)
			}
		}
	}()
}

func (c *refresher[T]) Close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.mu.Unlock()
	c.cancel()
	c.wg.Wait()
}

func (c *refresher[T]) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}
