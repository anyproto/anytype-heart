package filesync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-store/query"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
)

func (s *fileSync) DeleteFile(objectId string, fileId domain.FullFileId) error {
	if s.cfg.IsLocalOnlyMode() {
		return nil
	}

	return s.process(objectId, func(exists bool, info FileInfo) (FileInfo, bool, error) {
		if exists {
			info.State = FileStatePendingDeletion
			return info, true, nil
		}

		info = FileInfo{
			FileId:      fileId.FileId,
			SpaceId:     fileId.SpaceId,
			ObjectId:    objectId,
			State:       FileStatePendingDeletion,
			ScheduledAt: time.Now(),
		}
		return info, true, nil
	})
}

func (s *fileSync) runDeleter() {
	for {
		select {
		case <-s.loopCtx.Done():
			return
		default:
		}

		err := s.processNextToDelete(s.loopCtx)
		if err != nil && !errors.Is(err, filequeue.ErrClosed) {
			log.Error("process next to delete", zap.Error(err))
		}
	}
}

func (s *fileSync) processNextToDelete(ctx context.Context) error {
	item, err := s.queue.GetNextScheduled(ctx, filequeue.GetNextScheduledRequest[FileInfo]{
		Subscribe:   true,
		StoreFilter: filterByState(FileStatePendingDeletion),
		StoreOrder: &query.SortField{
			Field:   "scheduledAt",
			Path:    []string{"scheduledAt"},
			Reverse: false,
		},
		Filter: func(info FileInfo) bool {
			return info.State == FileStatePendingDeletion
		},
		ScheduledAt: func(info FileInfo) time.Time {
			return info.ScheduledAt
		},
	})
	if err != nil {
		return fmt.Errorf("get next scheduled item: %w", err)
	}

	next, err := s.processDeletion(ctx, item)

	releaseErr := s.queue.ReleaseAndUpdate(item.ObjectId, next)

	return errors.Join(releaseErr, err)
}

func (s *fileSync) processDeletion(ctx context.Context, it FileInfo) (FileInfo, error) {
	err := s.rpcStore.DeleteFiles(ctx, it.SpaceId, it.FileId)
	if err != nil {
		it = it.Reschedule()
		return it, err
	}

	mngr, err := s.limitManager.getSpace(ctx, it.SpaceId)
	if err == nil {
		mngr.deallocateFile(it.Key())
	} else {
		log.Error("processDeletion: get space limit manager", zap.Error(err))
	}

	it.State = FileStateDeleted
	return it, nil
}
