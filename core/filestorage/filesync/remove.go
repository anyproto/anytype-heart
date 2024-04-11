package filesync

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/queue"
)

func (f *fileSync) DeleteFile(objectId string, fileId domain.FullFileId) error {
	it := &QueueItem{
		ObjectId: objectId,
		SpaceId:  fileId.SpaceId,
		FileId:   fileId.FileId,
	}
	err := it.Validate()
	if err != nil {
		return fmt.Errorf("validate queue item: %w", err)
	}
	err = f.removeFromUploadingQueues(it)
	if err != nil {
		return fmt.Errorf("remove from uploading queues: %w", err)
	}
	return f.deletionQueue.Add(it)
}

func (f *fileSync) deletionHandler(ctx context.Context, it *QueueItem) (queue.Action, error) {
	fileId := domain.FullFileId{
		SpaceId: it.SpaceId,
		FileId:  it.FileId,
	}
	err := f.deleteFile(ctx, fileId)
	if err != nil {
		log.Error("remove file error", zap.String("fileId", fileId.FileId.String()), zap.Error(err))
		addErr := f.retryDeletionQueue.Add(it)
		if addErr != nil {
			return queue.ActionRetry, fmt.Errorf("add to removing retry queue: %w", addErr)
		}
		return queue.ActionDone, fmt.Errorf("remove file: %w", err)
	}
	f.updateSpaceUsageInformation(fileId.SpaceId)
	err = f.removeFromDeletionQueues(it)
	if err != nil {
		log.Error("remove from deletion queues", zap.String("fileId", it.FileId.String()), zap.Error(err))
	}
	return queue.ActionDone, nil
}

func (f *fileSync) retryDeletionHandler(ctx context.Context, it *QueueItem) (queue.Action, error) {
	fileId := domain.FullFileId{
		SpaceId: it.SpaceId,
		FileId:  it.FileId,
	}
	err := f.deleteFile(ctx, fileId)
	if err != nil {
		return queue.ActionRetry, fmt.Errorf("remove file: %w", err)
	}
	f.updateSpaceUsageInformation(fileId.SpaceId)
	err = f.removeFromDeletionQueues(it)
	if err != nil {
		log.Error("remove from deletion queues", zap.String("fileId", it.FileId.String()), zap.Error(err))
	}
	return queue.ActionDone, nil
}

func (f *fileSync) DeleteFileSynchronously(fileId domain.FullFileId) (err error) {
	err = f.deleteFile(context.Background(), fileId)
	if err != nil {
		return fmt.Errorf("remove file: %w", err)
	}
	f.updateSpaceUsageInformation(fileId.SpaceId)
	return
}

func (f *fileSync) deleteFile(ctx context.Context, fileId domain.FullFileId) error {
	log.Info("removing file", zap.String("fileId", fileId.FileId.String()))
	err := f.rpcStore.DeleteFiles(ctx, fileId.SpaceId, fileId.FileId)
	if err != nil {
		return err
	}
	log.Warn("file deleted", zap.String("fileId", fileId.FileId.String()))
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
