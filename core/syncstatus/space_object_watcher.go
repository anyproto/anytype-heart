package syncstatus

import (
	"context"

	"github.com/anytypeio/any-sync/commonspace/syncstatus"

	"github.com/anytypeio/go-anytype-middleware/space"
)

type spaceObjectWatcher struct {
	spaceService   space.Service
	updateReceiver syncstatus.UpdateReceiver

	watcher syncstatus.StatusWatcher
}

func NewSpaceObjectWatcher(
	spaceService space.Service,
	updateReceiver syncstatus.UpdateReceiver,
) RunnableWatcher {
	return &spaceObjectWatcher{
		spaceService:   spaceService,
		updateReceiver: updateReceiver,
	}
}

func (w *spaceObjectWatcher) Run(ctx context.Context) error {
	res, err := w.spaceService.AccountSpace(ctx)
	if err != nil {
		return err
	}

	w.watcher = res.SyncStatus().(syncstatus.StatusWatcher)
	w.watcher.SetUpdateReceiver(w.updateReceiver)

	return nil
}

func (w *spaceObjectWatcher) Watch(id string) error {
	return w.watcher.Watch(id)
}

func (w *spaceObjectWatcher) Unwatch(id string) {
	w.watcher.Unwatch(id)
}
