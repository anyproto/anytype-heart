package syncstatus

import (
	"context"

	"github.com/anytypeio/any-sync/commonspace/syncstatus"

	"github.com/anytypeio/go-anytype-middleware/space"
)

type objectWatcher struct {
	spaceService   space.Service
	updateReceiver syncstatus.UpdateReceiver

	watcher syncstatus.StatusWatcher
}

func newObjectWatcher(
	spaceService space.Service,
	updateReceiver syncstatus.UpdateReceiver,
) *objectWatcher {
	return &objectWatcher{
		spaceService:   spaceService,
		updateReceiver: updateReceiver,
	}
}

func (w *objectWatcher) run(ctx context.Context) error {
	res, err := w.spaceService.AccountSpace(ctx)
	if err != nil {
		return err
	}

	w.watcher = res.SyncStatus().(syncstatus.StatusWatcher)
	w.watcher.SetUpdateReceiver(w.updateReceiver)

	return nil
}

func (w *objectWatcher) Watch(id string) error {
	return w.watcher.Watch(id)
}

func (w *objectWatcher) Unwatch(id string) {
	w.watcher.Unwatch(id)
}
