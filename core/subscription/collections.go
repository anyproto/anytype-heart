package subscription

import (
	"sync"
)

// collectionWatcher feeds a collection's live membership into a sub's id
// scope. The collection editor broadcasts full id lists with blocking sends
// from its after-apply hook while holding the collection object's lock — the
// same lock UnsubscribeFromCollection needs — so a registered channel must
// be drained until that unsubscribe closes it, on every path.
type collectionWatcher struct {
	svc          *service
	sub          *coreSub
	collectionId string
	subId        string
	ch           <-chan []string
	done         chan struct{}
	wg           sync.WaitGroup

	mu      sync.Mutex
	started bool
	stopped bool
}

func newCollectionWatcher(svc *service, sub *coreSub, collectionId, subId string, ch <-chan []string) *collectionWatcher {
	return &collectionWatcher{
		svc:          svc,
		sub:          sub,
		collectionId: collectionId,
		subId:        subId,
		ch:           ch,
		done:         make(chan struct{}),
	}
}

// start launches the consumer; called once the sub is installed (updates
// sent before that wait in the channel — the editor blocks briefly at worst)
func (w *collectionWatcher) start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started || w.stopped {
		return
	}
	w.started = true
	w.wg.Add(1)
	go w.run()
}

func (w *collectionWatcher) run() {
	defer w.wg.Done()
	for {
		select {
		case <-w.done:
			return
		case ids, ok := <-w.ch:
			if !ok {
				return
			}
			// nil on the early-fail drain path: the sub never got a space
			// (written by the same goroutine that launched this drain)
			if space := w.sub.space; space != nil {
				space.setScope(w.sub, ids)
			}
		}
	}
}

// stop unsubscribes first — closing the channel upstream — so the consumer
// drains any in-flight broadcast instead of leaving the editor blocked on a
// send, then waits the goroutine out. On stop-before-start paths (install
// failure, lost subId race) a drain consumer is launched first: an editor
// broadcast may already be blocked on the unbuffered channel while holding
// the collection object's lock, which UnsubscribeFromCollection needs —
// without a consumer that is a permanent deadlock. Idempotent.
func (w *collectionWatcher) stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return
	}
	w.stopped = true
	if !w.started {
		w.started = true
		w.wg.Add(1)
		go w.run()
	}
	w.mu.Unlock()

	if w.svc.collectionService != nil {
		if err := w.svc.collectionService.UnsubscribeFromCollection(w.collectionId, w.subId); err != nil {
			// the channel may still be registered: leave the drain consumer
			// running (it exits if the channel ever closes) — a leaked
			// goroutine beats an editor wedged on a blocking send into a
			// channel nobody reads
			log.Warnf("subscription %s: unsubscribe from collection %s failed, leaving drain running: %v", w.subId, w.collectionId, err)
			return
		}
	}
	close(w.done)
	w.wg.Wait()
}
