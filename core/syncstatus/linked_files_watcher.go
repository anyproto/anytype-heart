package syncstatus

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
)

type linkedFilesWatcher struct {
	spaceService       space.Service
	fileStatusRegistry *fileStatusRegistry

	sync.Mutex
	linkedFilesSummary map[string]pb.EventStatusThreadCafePinStatus
	linkedFilesCloseCh map[string]chan struct{}
}

func newLinkedFilesWatcher(
	spaceService space.Service,
	fileStatusRegistry *fileStatusRegistry,
) *linkedFilesWatcher {
	return &linkedFilesWatcher{
		linkedFilesSummary: make(map[string]pb.EventStatusThreadCafePinStatus),
		linkedFilesCloseCh: make(map[string]chan struct{}),
		spaceService:       spaceService,
		fileStatusRegistry: fileStatusRegistry,
	}
}

func (w *linkedFilesWatcher) close() {
	w.Lock()
	defer w.Unlock()

	for key, closeCh := range w.linkedFilesCloseCh {
		close(closeCh)
		delete(w.linkedFilesCloseCh, key)
	}
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

	w.Lock()
	closeCh, ok := w.linkedFilesCloseCh[parentObjectID]
	if ok {
		close(closeCh)
	}
	closeCh = make(chan struct{})
	w.linkedFilesCloseCh[parentObjectID] = closeCh
	w.Unlock()

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
		case FileStatusUnknown, FileStatusSyncing:
			summary.Pinning++
		case FileStatusLimited:
			summary.Failed++
		case FileStatusSynced:
			summary.Pinned++
		}
	}

	w.Lock()
	w.linkedFilesSummary[parentObjectID] = summary
	w.Unlock()
}

func (w *linkedFilesWatcher) UnwatchLinkedFiles(parentObjectID string) {
	w.Lock()
	defer w.Unlock()

	if ch, ok := w.linkedFilesCloseCh[parentObjectID]; ok {
		close(ch)
		delete(w.linkedFilesCloseCh, parentObjectID)
	}
}
