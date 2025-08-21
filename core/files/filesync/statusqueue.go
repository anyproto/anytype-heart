package filesync

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

func makeStatusUpdateItem() *statusUpdateItem {
	return &statusUpdateItem{}
}

func (s *fileSync) statusUpdateHandler(ctx context.Context, it *statusUpdateItem) (persistentqueue.Action, error) {
	s.pendingDeletionsLock.Lock()
	_, toDelete := s.pendingDeletions[it.FileObjectId]
	s.pendingDeletionsLock.Unlock()

	for _, cb := range s.onStatusUpdated {
		err := cb(it.FileObjectId, domain.FullFileId{FileId: domain.FileId(it.FileId), SpaceId: it.SpaceId}, filesyncstatus.Status(it.Status))
		if err != nil {
			if isObjectDeletedError(err) {
				toDelete = true
				continue
			} else {
				log.Warn("on status update callback failed",
					zap.String("spaceId", it.SpaceId),
					zap.String("fileObjectId", it.FileObjectId),
					zap.String("fileId", it.FileId),
					zap.Error(err))
			}
			return persistentqueue.ActionRetry, err
		}
	}

	if toDelete {
		err := s.DeleteFile(it.FileObjectId, domain.FullFileId{FileId: domain.FileId(it.FileId), SpaceId: it.SpaceId})
		if err != nil {
			return persistentqueue.ActionRetry, fmt.Errorf("status update handler: delete file: %w", err)
		}
		s.pendingDeletionsLock.Lock()
		delete(s.pendingDeletions, it.FileObjectId)
		s.pendingDeletionsLock.Unlock()
	}

	return persistentqueue.ActionDone, nil
}
