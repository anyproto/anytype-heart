package filesync

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
)

func (f *fileSync) RemoveFile(fileId domain.FullFileId) error {
	err := f.removeFromUploadingQueues(fileId)
	if err != nil {
		return fmt.Errorf("remove from uploading queues: %w", err)
	}
	return f.removingQueue.add(f.loopCtx, &QueueItem{
		SpaceId: fileId.SpaceId,
		FileId:  fileId.FileId,
	})
}

func (f *fileSync) removeLoop() {
	for {
		select {
		case <-f.loopCtx.Done():
			return
		default:
		}

		it, err := f.removingQueue.getNext(f.loopCtx)
		if err != nil {
			log.Warn("can't get next file to upload", zap.Error(err))
			continue
		}
		err = f.tryToRemove(it)
		if err != nil {
			log.Warn("can't remove file", zap.String("fileId", it.FileId.String()), zap.Error(err))
			continue
		}
	}
}
func (f *fileSync) tryToRemove(it *QueueItem) error {
	spaceID, fileId := it.SpaceId, it.FileId
	err := f.removeFile(f.loopCtx, spaceID, fileId)
	if err != nil {
		addErr := f.removingQueue.add(f.loopCtx, it)
		if addErr != nil {
			log.Error("can't add file back to removing queue", zap.Error(addErr))
		}
		return fmt.Errorf("remove file: %w", err)
	}

	err = f.removingQueue.remove(it.FullFileId())
	if err != nil {
		return fmt.Errorf("mark remove task as done: %w", err)
	}
	f.updateSpaceUsageInformation(spaceID)

	return nil
}

func (f *fileSync) RemoveSynchronously(spaceId string, fileId domain.FileId) (err error) {
	err = f.removeFile(context.Background(), spaceId, fileId)
	if err != nil {
		return fmt.Errorf("remove file: %w", err)
	}
	f.updateSpaceUsageInformation(spaceId)
	return
}

func (f *fileSync) removeFile(ctx context.Context, spaceId string, fileId domain.FileId) (err error) {
	log.Info("removing file", zap.String("fileId", fileId.String()))
	return f.rpcStore.DeleteFiles(ctx, spaceId, fileId)
}
