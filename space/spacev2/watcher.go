package spacev2

import (
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/space/dedupqueue"
)

// statusApplier receives coalesced SpaceView status updates. Implemented by
// the service; the watcher itself carries no lifecycle logic (unidirectional
// spine stays a thin pipe).
type statusApplier interface {
	onSpaceStatusUpdated(status spaceViewStatus)
}

type spaceWatcher struct {
	sub     *spaceSubscription
	queue   *dedupqueue.DedupQueue
	applier statusApplier
}

func newSpaceWatcher(techSpaceId string, service subscription.Service, applier statusApplier) *spaceWatcher {
	w := &spaceWatcher{
		queue:   dedupqueue.New(0),
		applier: applier,
	}
	w.sub = newSpaceSubscription(
		service,
		techSpaceId,
		func(sub *spaceViewObjectSubscription) {
			sub.Iterate(func(id string, status spaceViewStatus) bool {
				w.enqueue(status)
				return true
			})
		},
		w.enqueue,
	)
	return w
}

// enqueue coalesces bursts per space id: only the latest pending status for a
// space is applied (dedupqueue Replace semantics).
func (w *spaceWatcher) enqueue(status spaceViewStatus) {
	w.queue.Replace(status.spaceId, func() {
		w.applier.onSpaceStatusUpdated(status)
	})
}

func (w *spaceWatcher) Run() error {
	w.queue.Run()
	return w.sub.Run()
}

func (w *spaceWatcher) Close() error {
	w.sub.Close()
	return w.queue.Close()
}
