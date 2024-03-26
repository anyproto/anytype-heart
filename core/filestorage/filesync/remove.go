package filesync

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	queue2 "github.com/anyproto/anytype-heart/core/queue"
)

func (f *fileSync) RemoveFile(fileId domain.FullFileId) error {
	err := f.removeFromUploadingQueues(&QueueItem{SpaceId: fileId.SpaceId, FileId: fileId.FileId})
	if err != nil {
		return fmt.Errorf("remove from uploading queues: %w", err)
	}
	return f.removingQueue.Add(&QueueItem{
		SpaceId: fileId.SpaceId,
		FileId:  fileId.FileId,
	})
}

func (f *fileSync) removingHandler(ctx context.Context, it *QueueItem) (queue2.Action, error) {
	spaceID, fileId := it.SpaceId, it.FileId
	err := f.removeFile(f.loopCtx, spaceID, fileId)
	if err != nil {
		return queue2.ActionRetry, fmt.Errorf("remove file: %w", err)
	}
	f.updateSpaceUsageInformation(spaceID)

	return queue2.ActionDone, nil
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
