package filesync

import (
	"context"
	"errors"
	"time"

	"github.com/anyproto/any-store/query"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
)

func (s *fileSync) runLimitedUploader(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := s.processLimited(ctx)
			if err != nil && !errors.Is(err, filequeue.ErrClosed) {
				log.Error("process next limited upload item", zap.Error(err))
			}
		}
	}
}

func (s *fileSync) processLimited(ctx context.Context) error {
	for update := range s.limitManager.updateCh {
		freeSpace := update.freeSpace()
		for {
			nextFreeSpace, err := s.getLimitedFile(ctx, update.spaceId, freeSpace)
			if err != nil {
				break
			}
			freeSpace = nextFreeSpace
		}
	}

	return nil
}

func (s *fileSync) getLimitedFile(ctx context.Context, spaceId string, freeSpace int) (int, error) {
	item, err := s.queue.GetNextScheduled(ctx, filequeue.GetNextScheduledRequest[FileInfo]{
		Subscribe: false, // Do not subscribe, just return error if no rows found
		StoreFilter: query.And{
			filterByState(FileStateLimited),
			filterBySpaceId(spaceId),
			filterByBytesToUpload(freeSpace),
		},
		StoreOrder: orderByScheduledAt(),
		Filter: func(info FileInfo) bool {
			return info.State == FileStateLimited && info.SpaceId == spaceId && info.BytesToUploadOrBind <= freeSpace
		},
		ScheduledAt: func(info FileInfo) time.Time {
			return info.ScheduledAt
		},
	})
	if errors.Is(err, filequeue.ErrNoRows) {
		return 0, err
	}
	if errors.Is(err, context.Canceled) {
		return 0, err
	}
	if err != nil {
		log.Error("process limited item", zap.Error(err))
	}

	next, err := s.processFilePendingUpload(ctx, item)
	if err == nil {
		freeSpace = max(0, freeSpace-item.BytesToUploadOrBind)
	}

	releaseErr := s.queue.ReleaseAndUpdate(next)

	return freeSpace, errors.Join(releaseErr, err)
}
