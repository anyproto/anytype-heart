package status

import (
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/filesync/filesyncstatus"
	"github.com/anytypeio/go-anytype-middleware/space"
)

type fileWatcher struct {
	spaceService      space.Service
	fileStatusWatcher filesyncstatus.StatusWatcher
}

func NewFileWatcher(
	spaceService space.Service,
	fileStatusWatcher filesyncstatus.StatusWatcher,
) Watcher {
	return &fileWatcher{
		spaceService:      spaceService,
		fileStatusWatcher: fileStatusWatcher,
	}
}

func (f *fileWatcher) Watch(id string) error {
	f.fileStatusWatcher.Watch(f.spaceService.AccountId(), id)
	return nil
}

func (f *fileWatcher) Unwatch(id string) {
	f.fileStatusWatcher.Unwatch(f.spaceService.AccountId(), id)
}
