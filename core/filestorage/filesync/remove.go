package filesync

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
)

func (f *fileSync) RemoveFile(spaceId string, fileId domain.FileId) (err error) {
	log.Info("add file to removing queue", zap.String("fileId", fileId.String()))
	defer func() {
		if err == nil {
			select {
			case f.removePingCh <- struct{}{}:
			default:
			}
		}
	}()
	err = f.queue.QueueRemove(spaceId, fileId)
	return
}

func (f *fileSync) removeLoop() {
	for {
		select {
		case <-f.loopCtx.Done():
			return
		case <-f.removePingCh:
		case <-time.After(loopTimeout):
		}
		f.removeOperation()

	}
}

func (f *fileSync) removeOperation() {
	for {
		fileId, err := f.tryToRemove()
		if err == errQueueIsEmpty {
			return
		}
		if err != nil {
			log.Warn("can't remove file", zap.String("fileId", fileId.String()), zap.Error(err))
			return
		}
		log.Warn("file removed", zap.String("fileId", fileId.String()))
	}
}

func (f *fileSync) tryToRemove() (domain.FileId, error) {
	it, err := f.queue.GetRemove()
	if err == errQueueIsEmpty {
		return "", errQueueIsEmpty
	}
	if err != nil {
		return "", fmt.Errorf("get remove task from queue: %w", err)
	}
	spaceID, fileId := it.SpaceId, it.FileId
	if err = f.removeFile(f.loopCtx, spaceID, fileId); err != nil {
		return fileId, fmt.Errorf("remove file: %w", err)
	}
	if err = f.queue.DoneRemove(spaceID, fileId); err != nil {
		return fileId, fmt.Errorf("mark remove task as done: %w", err)
	}
	f.updateSpaceUsageInformation(spaceID)

	return fileId, nil
}

func (f *fileSync) removeFile(ctx context.Context, spaceId string, fileId domain.FileId) (err error) {
	log.Info("removing file", zap.String("fileId", fileId.String()))
	return f.rpcStore.DeleteFiles(ctx, spaceId, fileId)
}
