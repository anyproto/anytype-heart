package treesyncer

import (
	"context"
	"sync"
)

type RefreshableComponent[T any] struct {
	action    func(ctx context.Context) T
	mu        sync.RWMutex
	onRefresh func(T)
	running   bool
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	closed    bool
}

func NewRefreshableComponent[T any](action func(ctx context.Context) T) *RefreshableComponent[T] {
	ctx, cancel := context.WithCancel(context.Background())
	return &RefreshableComponent[T]{
		action: action,
		ctx:    ctx,
		cancel: cancel,
	}
}

func (c *RefreshableComponent[T]) Refresh(onRefresh func(T)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.onRefresh = onRefresh
	if c.running {
		return
	}
	c.running = true
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer func() {
			c.mu.Lock()
			c.running = false
			c.mu.Unlock()
		}()
		result := c.action(c.ctx)
		c.mu.RLock()
		callback := c.onRefresh
		closed := c.closed
		c.mu.RUnlock()
		if !closed && callback != nil {
			callback(result)
		}
	}()
}

func (c *RefreshableComponent[T]) Close() {
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

func (c *RefreshableComponent[T]) IsRunning() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.running
}
