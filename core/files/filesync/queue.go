package filesync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-store/query"
	blocks "github.com/ipfs/go-block-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
)

func (s *fileSync) processFilePendingUpload(ctx context.Context, it FileInfo) (FileInfo, error) {
	blocksAvailability, err := s.checkBlocksAvailability(ctx, it)
	if err != nil {
		return it, fmt.Errorf("check blocks availability: %w", err)
	}

	it.BytesToUploadOrBind = blocksAvailability.bytesToUploadOrBind
	it.CidsToBind = blocksAvailability.cidsToBind

	spaceLimits, err := s.limitManager.getSpace(ctx, it.SpaceId)
	if err != nil {
		return it, fmt.Errorf("get space limits: %w", err)
	}

	allocateErr := spaceLimits.allocateFile(ctx, it.Key(), blocksAvailability.bytesToUploadOrBind)
	// TODO De-allocate if error is occurred
	if allocateErr != nil {
		it.State = FileStateLimited

		err = s.handleLimitReached(ctx, it)
		if err != nil {
			return it, fmt.Errorf("handle limit reached: %w", err)
		}
		return it, nil
	}

	if it.ObjectId != "" {
		err = s.updateStatus(it, filesyncstatus.Syncing)
		if isObjectDeletedError(err) {
			it.State = FileStatePendingDeletion
			return it, nil
		}
	}

	var totalBytesToUpload int
	err = s.walkFileBlocks(ctx, it.SpaceId, it.FileId, it.Variants, func(fileBlocks []blocks.Block) error {
		bytesToUpload, err := s.uploadOrBindBlocks(ctx, it, fileBlocks, blocksAvailability.cidsToBind)
		if err != nil {
			return fmt.Errorf("select blocks to upload: %w", err)
		}
		totalBytesToUpload += bytesToUpload
		return nil
	})

	// All cids should be bind at this time
	it.CidsToBind = nil

	if err != nil {
		if isNodeLimitReachedError(err) {
			it.State = FileStateLimited

			err = s.handleLimitReached(ctx, it)
			if err != nil {
				return it, fmt.Errorf("handle limit reached: %w", err)
			}
			return it, nil
		}
		return it, fmt.Errorf("walk file blocks: %w", err)
	}

	// Means that we only had to bind blocks
	if totalBytesToUpload == 0 {
		err := s.updateStatus(it, filesyncstatus.Synced)
		if err != nil {
			return it, fmt.Errorf("add to status update queue: %w", err)
		}
		it.State = FileStateDone
		return it, nil
	}

	it.State = FileStateUploading
	return it, nil
}

func (s *fileSync) handleLimitReached(ctx context.Context, it FileInfo) error {
	// Unbind file just in case
	err := s.rpcStore.DeleteFiles(ctx, it.SpaceId, it.FileId)
	if err != nil {
		log.Error("calculate limits: unbind off-limit file", zap.String("fileId", it.FileId.String()), zap.Error(err))
	}

	updateErr := s.updateStatus(it, filesyncstatus.Limited)
	if updateErr != nil {
		return fmt.Errorf("enqueue status update: %w", updateErr)
	}

	if it.AddedByUser && !it.Imported {
		s.sendLimitReachedEvent(it.SpaceId)
	}
	if it.Imported {
		s.addImportEvent(it.SpaceId)
	}
	return nil
}

func (s *fileSync) processFileUploading(ctx context.Context, it FileInfo) (FileInfo, error) {
	if len(it.CidsToUpload) == 0 {
		space, err := s.limitManager.getSpace(ctx, it.SpaceId)
		if err != nil {
			return it, fmt.Errorf("get space limits: %w", err)
		}
		space.markFileUploaded(it.Key())

		err = s.updateStatus(it, filesyncstatus.Synced)
		if err != nil {
			return it, err
		}
		it.State = FileStateDone
		return it, nil
	}

	return it, nil
}

func (s *fileSync) processFilePendingDeletion(ctx context.Context, it FileInfo) (FileInfo, error) {
	log.Info("removing file", zap.String("fileId", it.FileId.String()))
	err := s.rpcStore.DeleteFiles(ctx, it.SpaceId, it.FileId)
	if err != nil {
		return it, err
	}
	log.Warn("file deleted", zap.String("fileId", it.FileId.String()))

	it.State = FileStateDeleted
	return it, nil
}

func (s *fileSync) resetUploadingStatus(ctx context.Context) error {
	item, err := s.queue.GetNext(ctx, filequeue.GetNextRequest[FileInfo]{
		Subscribe: false,
		StoreFilter: query.Key{
			Path:   []string{"state"},
			Filter: query.NewComp(query.CompOpEq, int(FileStateUploading)),
		},
		Filter: func(info FileInfo) bool {
			return info.State == FileStateUploading
		},
	})
	if err != nil {
		return fmt.Errorf("get next scheduled item: %w", err)
	}

	item.State = FileStatePendingUpload
	item.ScheduledAt = time.Now()

	releaseErr := s.queue.Release(item)

	return errors.Join(releaseErr, err)
}

func (s *fileSync) runUploader(ctx context.Context) {

	for {
		err := s.resetUploadingStatus(ctx)
		if errors.Is(err, filequeue.ErrNoRows) {
			break
		}
		if err != nil {
			log.Error("reset uploading status", zap.Error(err))
		}
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := s.processNextPendingUploadItem(ctx, FileStatePendingUpload)
			if err != nil {
				log.Error("process next pending upload item", zap.Error(err))
			}
		}
	}
}

func (s *fileSync) runLimitedUploader(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := s.processLimited(ctx)
			if err != nil {
				log.Error("process next limited upload item", zap.Error(err))
			}
		}
	}
}

func (s *fileSync) printQueue() error {
	items, err := s.queue.List()
	if err != nil {
		return err
	}

	for _, item := range items {
		fmt.Printf("%#v\n", item)
	}
	return nil
}

func (s *fileSync) processNextPendingUploadItem(ctx context.Context, state FileState) error {
	item, err := s.queue.GetNextScheduled(ctx, filequeue.GetNextScheduledRequest[FileInfo]{
		Subscribe: true,
		StoreFilter: query.Key{
			Path:   []string{"state"},
			Filter: query.NewComp(query.CompOpEq, int(state)),
		},
		StoreOrder: &query.SortField{
			Field:   "scheduledAt",
			Path:    []string{"scheduledAt"},
			Reverse: false,
		},
		Filter: func(info FileInfo) bool {
			return info.State == state
		},
		ScheduledAt: func(info FileInfo) time.Time {
			return info.ScheduledAt
		},
	})
	if err != nil {
		return fmt.Errorf("get next scheduled item: %w", err)
	}

	next, err := s.processFilePendingUpload(ctx, item)

	releaseErr := s.queue.Release(next)

	return errors.Join(releaseErr, err)
}

type limitUpdate struct {
	spaceId string
	limit   int
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
			query.Key{
				Path:   []string{"state"},
				Filter: query.NewComp(query.CompOpEq, int(FileStateLimited)),
			},
			query.Key{
				Path:   []string{"spaceId"},
				Filter: query.NewComp(query.CompOpEq, spaceId),
			},
			query.Key{
				Path:   []string{"bytesToUploadOrBind"},
				Filter: query.NewComp(query.CompOpLte, freeSpace),
			},
		},
		StoreOrder: &query.SortField{
			Field:   "scheduledAt",
			Path:    []string{"scheduledAt"},
			Reverse: false,
		},
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

	releaseErr := s.queue.Release(next)

	nextFreeSpace := max(0, freeSpace-item.BytesToUploadOrBind)
	return nextFreeSpace, errors.Join(releaseErr, err)
}

func (s *fileSync) uploadLimited(ctx context.Context) (bool, error) {
	limitUpdated := make(chan int)
	nextCtx, nextCtxCancel := context.WithCancel(ctx)

	go func() {
		defer nextCtxCancel()

		select {
		case <-limitUpdated:
		case <-ctx.Done():
		}
	}()

	item, err := s.queue.GetNextScheduled(nextCtx, filequeue.GetNextScheduledRequest[FileInfo]{
		Subscribe: true,
		StoreFilter: query.Key{
			Path:   []string{"state"},
			Filter: query.NewComp(query.CompOpEq, int(FileStatePendingUpload)),
		},
		StoreOrder: &query.SortField{
			Field:   "scheduledAt",
			Path:    []string{"scheduledAt"},
			Reverse: false,
		},
		Filter: func(info FileInfo) bool {
			return info.State == FileStateLimited
		},
		ScheduledAt: func(info FileInfo) time.Time {
			return info.ScheduledAt
		},
	})

	var retry bool
	if errors.Is(err, context.Canceled) {
		select {
		case <-ctx.Done():
			return false, err
		default:
		}

		select {
		case <-nextCtx.Done():
			retry = true
		default:
		}
	}
	if retry {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("get next scheduled item: %w", err)
	}

	next, err := s.processFilePendingUpload(ctx, item)

	releaseErr := s.queue.Release(next)

	return false, errors.Join(releaseErr, err)
}

func (s *fileSync) process(id string, proc func(exists bool, info FileInfo) (FileInfo, error)) error {
	item, err := s.queue.GetById(id)
	if err != nil && !errors.Is(err, filequeue.ErrNotFound) {
		return fmt.Errorf("get item: %w", err)
	}
	exists := !errors.Is(err, filequeue.ErrNotFound)

	next, err := proc(exists, item)
	if err != nil {
		return errors.Join(s.queue.Release(item), fmt.Errorf("process item: %w", err))
	}
	return s.queue.Release(next)
}
