package indexer

import (
	"context"
	"sync"
)

// maxConcurrentSpaceReindexFor returns how many spaces may run the
// outdated-objects reindex pass at once on the given platform. Each pass
// cold-builds every outdated object into the space's object cache and relies on
// the cache TTL to release it, so with all spaces starting the pass on load the
// resident set peaks at hundreds of MB (see docs/reindex-peak-heap-handoff.md).
// Builds are storage-read-bound, so a small overlap keeps throughput while
// capping the peak; mobile gets half the slots of desktop.
func maxConcurrentSpaceReindexFor(goos string) int {
	if goos == "ios" || goos == "android" {
		return 2
	}
	return 4
}

// reindexLimiter bounds cross-space concurrency of the outdated-objects
// reindex pass. Waiters queue FIFO, except that a freed slot is granted first
// to a waiting space the user currently has objects open in (openedSpaceIds,
// evaluated at grant time), so the visible space's index freshness is not stuck
// behind big background spaces.
type reindexLimiter struct {
	mu             sync.Mutex
	width          int
	active         int
	waiters        []*reindexWaiter
	openedSpaceIds func() map[string]struct{}
}

type reindexWaiter struct {
	spaceId string
	ready   chan struct{}
	granted bool
}

func newReindexLimiter(width int, openedSpaceIds func() map[string]struct{}) *reindexLimiter {
	return &reindexLimiter{
		width:          width,
		openedSpaceIds: openedSpaceIds,
	}
}

// acquire blocks until a slot is granted or ctx is done; it reports whether the
// caller holds a slot and must release it.
func (l *reindexLimiter) acquire(ctx context.Context, spaceId string) bool {
	if ctx.Err() != nil {
		return false
	}
	l.mu.Lock()
	if l.active < l.width {
		l.active++
		l.mu.Unlock()
		return true
	}
	w := &reindexWaiter{spaceId: spaceId, ready: make(chan struct{})}
	l.waiters = append(l.waiters, w)
	l.mu.Unlock()

	select {
	case <-w.ready:
		return true
	case <-ctx.Done():
		l.mu.Lock()
		defer l.mu.Unlock()
		if w.granted {
			// the slot was handed over concurrently with cancellation: pass it on
			l.releaseLocked()
			return false
		}
		for i, waiter := range l.waiters {
			if waiter == w {
				l.waiters = append(l.waiters[:i], l.waiters[i+1:]...)
				break
			}
		}
		return false
	}
}

func (l *reindexLimiter) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.releaseLocked()
}

// releaseLocked hands the slot to the preferred waiter (opened space first,
// then FIFO) without decrementing active, or frees the slot when nobody waits.
func (l *reindexLimiter) releaseLocked() {
	if len(l.waiters) == 0 {
		l.active--
		return
	}
	next := 0
	if l.openedSpaceIds != nil {
		opened := l.openedSpaceIds()
		for i, w := range l.waiters {
			if _, ok := opened[w.spaceId]; ok {
				next = i
				break
			}
		}
	}
	w := l.waiters[next]
	l.waiters = append(l.waiters[:next], l.waiters[next+1:]...)
	w.granted = true
	close(w.ready)
}

// waitingCount reports how many acquirers are queued; used by tests to
// synchronize on the queue state.
func (l *reindexLimiter) waitingCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.waiters)
}
