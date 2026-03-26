package space

import (
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/space/dedupqueue"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type spaceViewUpdater interface {
	onSpaceStatusUpdated(spaceViewStatus)
}

type spaceWatcher struct {
	sub   *spaceSubscription
	queue *dedupqueue.DedupQueue
}

func newSpaceWatcher(techSpaceId string, service subscription.Service, updater spaceViewUpdater, onInitialSpacesCount func(count int32)) *spaceWatcher {
	dedupQueue := dedupqueue.New(0)
	spaceSub := newSpaceSubscription(
		service,
		techSpaceId,
		func(sub *spaceViewObjectSubscription) {
			var count int32
			sub.Iterate(func(id string, status spaceViewStatus) bool {
				count++
				dedupQueue.Replace(id, func() {
					updater.onSpaceStatusUpdated(status)
				})
				return true
			})
			if onInitialSpacesCount != nil {
				onInitialSpacesCount(count)
			}
		},
		func(status spaceViewStatus) {
			dedupQueue.Replace(status.spaceId, func() {
				updater.onSpaceStatusUpdated(status)
			})
		})
	return &spaceWatcher{sub: spaceSub, queue: dedupQueue}
}

func (w *spaceWatcher) Run() error {
	w.queue.Run()
	return w.sub.Run()
}

func (w *spaceWatcher) Close() error {
	w.sub.Close()
	return w.queue.Close()
}

func statusToInfo(status spaceViewStatus) spaceinfo.SpacePersistentInfo {
	persistentInfo := spaceinfo.NewSpacePersistentInfo(status.spaceId)
	persistentInfo.SetAccountStatus(status.accountStatus).
		SetAclHeadId(status.aclHeadId).
		SetEncodedKey(status.guestKey)
	return persistentInfo
}
