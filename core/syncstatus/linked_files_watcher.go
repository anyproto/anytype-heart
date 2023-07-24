package syncstatus

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
)

type linkedFilesSummary struct {
	pinStatus pb.EventStatusThreadCafePinStatus
	isUpdated bool
}

type linkedFilesWatcher struct {
	spaceService       space.Service
	fileStatusRegistry *fileStatusRegistry

	sync.Mutex
	linkedFilesSummaries map[string]linkedFilesSummary
	linkedFilesCloseCh   map[string]chan struct{}
}

func newLinkedFilesWatcher(
	spaceService space.Service,
	fileStatusRegistry *fileStatusRegistry,
) *linkedFilesWatcher {
	return &linkedFilesWatcher{
		linkedFilesSummaries: make(map[string]linkedFilesSummary),
		linkedFilesCloseCh:   make(map[string]chan struct{}),
		spaceService:         spaceService,
		fileStatusRegistry:   fileStatusRegistry,
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

func (w *linkedFilesWatcher) GetLinkedFilesSummary(parentObjectID string) linkedFilesSummary {
	w.Lock()
	defer w.Unlock()
	return w.linkedFilesSummaries[parentObjectID]
}

func (w *linkedFilesWatcher) WatchLinkedFiles(spaceID string, parentObjectID string, filesGetter func() []string) {
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
		w.updateLinkedFilesSummary(spaceID, parentObjectID, filesGetter)
		ticker := time.NewTicker(5 * time.Second)
		for {
			select {
			case <-closeCh:
				return
			case <-ticker.C:
				w.updateLinkedFilesSummary(spaceID, parentObjectID, filesGetter)
			}
		}
	}()
}

func (w *linkedFilesWatcher) updateLinkedFilesSummary(spaceID string, parentObjectID string, filesGetter func() []string) {
	// TODO Cache linked files list?
	fileIDs := filesGetter()

	var pinStatus pb.EventStatusThreadCafePinStatus
	for _, fileID := range fileIDs {
		status, err := w.fileStatusRegistry.GetFileStatus(context.Background(), spaceID, fileID)
		if errors.Is(err, domain.ErrFileNotFound) {
			continue
		}
		if err != nil {
			log.Desugar().Error("can't get status of dependent file", zap.String("fileID", fileID), zap.Error(err))
		}

		switch status {
		case FileStatusUnknown, FileStatusSyncing:
			pinStatus.Pinning++
		case FileStatusLimited:
			pinStatus.Failed++
		case FileStatusSynced:
			pinStatus.Pinned++
		}
	}

	updated := true
	w.Lock()
	if summary, exists := w.linkedFilesSummaries[parentObjectID]; exists {
		updated = summary.pinStatus != pinStatus
	}
	w.linkedFilesSummaries[parentObjectID] = linkedFilesSummary{pinStatus: pinStatus, isUpdated: updated}
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
