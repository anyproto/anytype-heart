package subscription

import (
	"sync"
)

// collectionWatcher feeds a collection's live membership into a sub's id
// scope. The collection editor broadcasts full id lists with blocking sends
// from its after-apply hook, so the watcher keeps receiving until the channel
// is closed by UnsubscribeFromCollection — never abandoning the channel while
// it is still registered.
type collectionWatcher struct {
	svc          *service
	sub          *coreSub
	collectionId string
	subId        string
	ch           <-chan []string
	done         chan struct{}
	wg           sync.WaitGroup
	started      bool
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
			w.sub.space.setScope(w.sub, ids)
		}
	}
}

// stop unsubscribes first — closing the channel upstream — so the consumer
// drains any in-flight broadcast instead of leaving the editor blocked on a
// send, then waits the goroutine out
func (w *collectionWatcher) stop() {
	if w.svc.collectionService != nil {
		if err := w.svc.collectionService.UnsubscribeFromCollection(w.collectionId, w.subId); err != nil {
			log.Warnf("subscription %s: unsubscribe from collection %s: %v", w.subId, w.collectionId, err)
		}
	}
	close(w.done)
	if w.started {
		w.wg.Wait()
	}
}
