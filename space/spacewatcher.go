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
	sub *spaceSubscription
	queue *dedupqueue.DedupQueue
}

func newSpaceWatcher(techSpaceId string, service subscription.Service, updater spaceViewUpdater) *spaceWatcher {
	dedupQueue := dedupqueue.New(0)
	spaceSub := newSpaceSubscription(
		service,
		techSpaceId,
		func(sub *spaceViewObjectSubscription) {
			sub.Iterate(func(id string, status spaceViewStatus) bool {
				dedupQueue.Replace(id, func() {
					updater.onSpaceStatusUpdated(status)
				})
				return true
			})
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
	persistentInfo.SetAccountStatus(spaceinfo.AccountStatus(status.accountStatus)).
		SetAclHeadId(status.aclHeadId).
		SetEncodedKey(status.guestKey)
	return persistentInfo
}