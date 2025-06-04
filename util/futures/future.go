package futures

import (
	"sync"
)

type Future[T any] struct {
	cond *sync.Cond

	ok    bool
	value T
	err   error
}

// New creates a value that should be resolved later. It's necessary to resolve a future eventually, otherwise there is
// a possibility of deadlock, when someone waits for never-resolving future.
func New[T any]() *Future[T] {
	return &Future[T]{
		cond: &sync.Cond{
			L: &sync.Mutex{},
		},
	}
}

func (f *Future[T]) Wait() (T, error) {
	f.cond.L.Lock()
	for !f.ok {
		f.cond.Wait()
	}
	f.cond.L.Unlock()

	return f.value, f.err
}

// Resolve sets value or error for future only once, all consequent calls to Resolve have no effect
func (f *Future[T]) Resolve(val T, err error) {
	f.cond.L.Lock()
	defer f.cond.L.Unlock()

	// Resolve once
	if f.ok {
		return
	}

	f.ok = true
	f.value = val
	f.err = err

	f.cond.Broadcast()
}

func (f *Future[T]) ResolveValue(val T) {
	f.Resolve(val, nil)
}

func (f *Future[T]) ResolveErr(err error) {
	var defaultValue T
	f.Resolve(defaultValue, err)
}
