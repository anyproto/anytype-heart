package filesync

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/queue"
)

func (f *fileSync) DeleteFile(fileId domain.FullFileId) error {
	err := f.removeFromUploadingQueues(&QueueItem{SpaceId: fileId.SpaceId, FileId: fileId.FileId})
	if err != nil {
		return fmt.Errorf("remove from uploading queues: %w", err)
	}
	return f.deletionQueue.Add(&QueueItem{
		SpaceId: fileId.SpaceId,
		FileId:  fileId.FileId,
	})
}

func (f *fileSync) deletionHandler(ctx context.Context, it *QueueItem) (queue.Action, error) {
	spaceID, fileId := it.SpaceId, it.FileId
	err := f.deleteFile(ctx, spaceID, fileId)
	if err != nil {
		log.Error("remove file error", zap.String("fileId", fileId.String()), zap.Error(err))
		addErr := f.retryDeletionQueue.Add(it)
		if addErr != nil {
			return queue.ActionRetry, fmt.Errorf("add to removing retry queue: %w", addErr)
		}
		return queue.ActionDone, fmt.Errorf("remove file: %w", err)
	}
	f.updateSpaceUsageInformation(spaceID)
	err = f.removeFromDeletionQueues(it)
	if err != nil {
		log.Error("remove from deletion queues", zap.String("fileId", it.FileId.String()), zap.Error(err))
	}
	return queue.ActionDone, nil
}

func (f *fileSync) retryDeletionHandler(ctx context.Context, it *QueueItem) (queue.Action, error) {
	spaceID, fileId := it.SpaceId, it.FileId
	err := f.deleteFile(ctx, spaceID, fileId)
	if err != nil {
		return queue.ActionRetry, fmt.Errorf("remove file: %w", err)
	}
	f.updateSpaceUsageInformation(spaceID)
	err = f.removeFromDeletionQueues(it)
	if err != nil {
		log.Error("remove from deletion queues", zap.String("fileId", it.FileId.String()), zap.Error(err))
	}
	return queue.ActionDone, nil
}

func (f *fileSync) DeleteFileSynchronously(spaceId string, fileId domain.FileId) (err error) {
	err = f.deleteFile(context.Background(), spaceId, fileId)
	if err != nil {
		return fmt.Errorf("remove file: %w", err)
	}
	f.updateSpaceUsageInformation(spaceId)
	return
}

func (f *fileSync) deleteFile(ctx context.Context, spaceId string, fileId domain.FileId) error {
	log.Info("removing file", zap.String("fileId", fileId.String()))
	err := f.rpcStore.DeleteFiles(ctx, spaceId, fileId)
	if err != nil {
		return err
	}
	log.Warn("file deleted", zap.String("fileId", fileId.String()))
	return nil
}

func (f *fileSync) removeFromDeletionQueues(item *QueueItem) error {
	err := f.deletionQueue.Remove(item.Key())
	if err != nil {
		return fmt.Errorf("remove upload task: %w", err)
	}
	err = f.retryDeletionQueue.Remove(item.Key())
	if err != nil {
		return fmt.Errorf("remove upload task from retrying queue: %w", err)
	}
	return nil
}
