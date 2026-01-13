package filesync

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
)

func (s *fileSync) updateStatus(it FileInfo, status filesyncstatus.Status) error {
	for _, cb := range s.onStatusUpdated {
		err := cb(it.ObjectId, domain.FullFileId{FileId: it.FileId, SpaceId: it.SpaceId}, status)
		if err != nil {
			if isObjectDeletedError(err) {
				err := s.DeleteFile(it.ObjectId, domain.FullFileId{FileId: it.FileId, SpaceId: it.SpaceId})
				if err != nil {
					return fmt.Errorf("status update handler: delete file: %w", err)
				}
				break
			} else {
				log.Warn("on status update callback failed",
					zap.String("spaceId", it.SpaceId),
					zap.String("fileObjectId", it.ObjectId),
					zap.String("fileId", it.FileId.String()),
					zap.Error(err))
			}
			return err
		}
	}
	return nil
}
