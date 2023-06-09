package filesync

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

func (f *fileSync) RemoveFile(spaceId, fileId string) (err error) {
	log.Info("add file to removing queue", zap.String("fileID", fileId))
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
		fileID, err := f.tryToRemove()
		if err == errQueueIsEmpty {
			return
		}
		if err != nil {
			log.Warn("can't remove file", zap.String("fileID", fileID), zap.Error(err))
			return
		}
	}
}

func (f *fileSync) tryToRemove() (string, error) {
	it, err := f.queue.GetRemove()
	if err == errQueueIsEmpty {
		return "", errQueueIsEmpty
	}
	if err != nil {
		return "", fmt.Errorf("get remove task from queue: %w", err)
	}
	spaceID, fileID := it.SpaceID, it.FileID
	if err = f.removeFile(f.loopCtx, spaceID, fileID); err != nil {
		return fileID, fmt.Errorf("remove file: %w", err)
	}
	if err = f.queue.DoneRemove(spaceID, fileID); err != nil {
		return fileID, fmt.Errorf("mark remove task as done: %w", err)
	}
	f.updateSpaceUsageInformation(spaceID)

	return fileID, nil
}

func (f *fileSync) removeFile(ctx context.Context, spaceId, fileId string) (err error) {
	log.Info("removing file", zap.String("fileID", fileId))
	return f.rpcStore.DeleteFiles(ctx, spaceId, fileId)
}
