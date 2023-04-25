package syncstatus

import (
	"context"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/filestorage/filesync/filesyncstatus"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/space"
)

type LinkedFilesWatcher interface {
	GetLinkedFilesSummary(parentObjectID string) pb.EventStatusThreadCafePinStatus
	WatchLinkedFiles(parentObjectID string, filesGetter func() []string)
	UnwatchLinkedFiles(parentObjectID string)

	app.ComponentRunnable
}

func NewLinkedFilesWatcher(
	spaceService space.Service,
	fileStatusRegistry filesyncstatus.Registry,
) LinkedFilesWatcher {
	return &linkedFilesWatcher{
		linkedFilesSummary: make(map[string]pb.EventStatusThreadCafePinStatus),
		linkedFilesCloseCh: make(map[string]chan struct{}),
		spaceService:       spaceService,
		fileStatusRegistry: fileStatusRegistry,
	}
}

type linkedFilesWatcher struct {
	spaceService       space.Service
	fileStatusRegistry filesyncstatus.Registry

	sync.Mutex
	linkedFilesSummary map[string]pb.EventStatusThreadCafePinStatus
	linkedFilesCloseCh map[string]chan struct{}
}

func (w *linkedFilesWatcher) Run(ctx context.Context) error {
	return nil
}

func (w *linkedFilesWatcher) Init(a *app.App) error {
	return nil
}

func (w *linkedFilesWatcher) Name() string {
	return "linked_files_watcher"
}

func (w *linkedFilesWatcher) Close(ctx context.Context) error {
	for _, closeCh := range w.linkedFilesCloseCh {
		close(closeCh)
	}
	return nil
}

func (w *linkedFilesWatcher) GetLinkedFilesSummary(parentObjectID string) pb.EventStatusThreadCafePinStatus {
	w.Lock()
	defer w.Unlock()
	return w.linkedFilesSummary[parentObjectID]
}

func (w *linkedFilesWatcher) WatchLinkedFiles(parentObjectID string, filesGetter func() []string) {
	if filesGetter == nil {
		return
	}

	closeCh, ok := w.linkedFilesCloseCh[parentObjectID]
	if ok {
		close(closeCh)
	}
	closeCh = make(chan struct{})
	w.linkedFilesCloseCh[parentObjectID] = closeCh

	go func() {
		w.updateLinkedFilesSummary(parentObjectID, filesGetter)
		ticker := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-closeCh:
				return
			case <-ticker.C:
				w.updateLinkedFilesSummary(parentObjectID, filesGetter)
			}
		}
	}()
}

func (w *linkedFilesWatcher) updateLinkedFilesSummary(parentObjectID string, filesGetter func() []string) {
	// TODO Cache linked files list?
	fileIDs := filesGetter()

	var summary pb.EventStatusThreadCafePinStatus
	for _, fileID := range fileIDs {
		status, err := w.fileStatusRegistry.GetFileStatus(context.Background(), w.spaceService.AccountId(), fileID)
		if err != nil {
			log.Desugar().Error("can't get status of dependent file", zap.String("fileID", fileID), zap.Error(err))
		}

		switch status {
		case syncstatus.StatusUnknown:
			summary.Pinning++
		case syncstatus.StatusNotSynced:
			summary.Pinning++
		case syncstatus.StatusSynced:
			summary.Pinned++
		}
	}

	w.Lock()
	w.linkedFilesSummary[parentObjectID] = summary
	w.Unlock()
}

func (w *linkedFilesWatcher) UnwatchLinkedFiles(parentObjectID string) {
	if ch, ok := w.linkedFilesCloseCh[parentObjectID]; ok {
		close(ch)
		delete(w.linkedFilesCloseCh, parentObjectID)
	}
}
