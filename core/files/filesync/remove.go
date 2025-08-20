package filesync

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

type deletionQueueItem struct {
	ObjectId string
	SpaceId  string
	FileId   domain.FileId
}

func makeDeletionQueueItem() *deletionQueueItem {
	return &deletionQueueItem{}
}

func (it *deletionQueueItem) Validate() error {
	if !it.FileId.Valid() {
		return fmt.Errorf("invalid file id")
	}
	return nil
}

func (it *deletionQueueItem) Key() string {
	return it.FileId.String()
}

func (s *fileSync) DeleteFile(objectId string, fileId domain.FullFileId) error {
	it := &deletionQueueItem{
		ObjectId: objectId,
		SpaceId:  fileId.SpaceId,
		FileId:   fileId.FileId,
	}
	err := it.Validate()
	if err != nil {
		return fmt.Errorf("validate queue item: %w", err)
	}
	err = s.removeFromUploadingQueues(it.ObjectId)
	if err != nil {
		return fmt.Errorf("remove from uploading queues: %w", err)
	}
	return s.deletionQueue.Add(it)
}

func (s *fileSync) deletionHandler(ctx context.Context, it *deletionQueueItem) (persistentqueue.Action, error) {
	fileId := domain.FullFileId{
		SpaceId: it.SpaceId,
		FileId:  it.FileId,
	}
	err := s.deleteFile(ctx, fileId)
	if err != nil {
		log.Error("remove file error", zap.String("fileId", fileId.FileId.String()), zap.Error(err))
		addErr := s.retryDeletionQueue.Add(it)
		if addErr != nil {
			return persistentqueue.ActionRetry, fmt.Errorf("add to removing retry queue: %w", addErr)
		}
		return persistentqueue.ActionDone, fmt.Errorf("remove file: %w", err)
	}
	err = s.removeFromDeletionQueues(it)
	if err != nil {
		log.Error("remove from deletion queues", zap.String("fileId", it.FileId.String()), zap.Error(err))
	}
	return persistentqueue.ActionDone, nil
}

func (s *fileSync) retryDeletionHandler(ctx context.Context, it *deletionQueueItem) (persistentqueue.Action, error) {
	fileId := domain.FullFileId{
		SpaceId: it.SpaceId,
		FileId:  it.FileId,
	}
	err := s.deleteFile(ctx, fileId)
	if err != nil {
		return persistentqueue.ActionRetry, fmt.Errorf("remove file: %w", err)
	}
	err = s.removeFromDeletionQueues(it)
	if err != nil {
		log.Error("remove from deletion queues", zap.String("fileId", it.FileId.String()), zap.Error(err))
	}
	return persistentqueue.ActionDone, nil
}

func (s *fileSync) DeleteFileSynchronously(fileId domain.FullFileId) (err error) {
	err = s.deleteFile(context.Background(), fileId)
	if err != nil {
		return fmt.Errorf("remove file: %w", err)
	}
	return
}

func (s *fileSync) deleteFile(ctx context.Context, fileId domain.FullFileId) error {
	log.Info("removing file", zap.String("fileId", fileId.FileId.String()))
	err := s.rpcStore.DeleteFiles(ctx, fileId.SpaceId, fileId.FileId)
	if err != nil {
		return err
	}
	log.Warn("file deleted", zap.String("fileId", fileId.FileId.String()))
	return nil
}

func (s *fileSync) removeFromDeletionQueues(item *deletionQueueItem) error {
	err := s.deletionQueue.Remove(item.Key())
	if err != nil {
		return fmt.Errorf("remove upload task: %w", err)
	}
	err = s.retryDeletionQueue.Remove(item.Key())
	if err != nil {
		return fmt.Errorf("remove upload task from retrying queue: %w", err)
	}
	return nil
}

func (s *fileSync) CancelDeletion(objectId string, fileId domain.FullFileId) error {
	it := &deletionQueueItem{
		ObjectId: objectId,
		SpaceId:  fileId.SpaceId,
		FileId:   fileId.FileId,
	}
	err := it.Validate()
	if err != nil {
		return fmt.Errorf("validate queue item: %w", err)
	}

	waitCh1, err := s.deletionQueue.RemoveWait(it.Key())
	if err != nil {
		return fmt.Errorf("remove from deletion queue: %w", err)
	}
	waitCh2, err := s.retryDeletionQueue.RemoveWait(it.Key())
	if err != nil {
		return fmt.Errorf("remove from retry deletion queue: %w", err)
	}

	select {
	case <-waitCh1:
	case <-s.loopCtx.Done():
	}

	select {
	case <-waitCh2:
	case <-s.loopCtx.Done():
	}

	return nil
}
