package filesync

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-store/query"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
)

type FileState int

const (
	FileStatePendingUpload FileState = iota
	FileStateUploading
	FileStateLimited
	FileStatePendingDeletion
	FileStateDone
	FileStateDeleted
)

type FileInfo struct {
	FileId      domain.FileId
	SpaceId     string
	ObjectId    string
	State       FileState
	ScheduledAt time.Time
	Variants    []domain.FileId
	AddedByUser bool
	Imported    bool

	BytesToUpload int
	CidsToUpload  map[cid.Cid]struct{}
}

func (i FileInfo) FullFileId() domain.FullFileId {
	return domain.FullFileId{
		FileId:  i.FileId,
		SpaceId: i.SpaceId,
	}
}

func (i FileInfo) Key() string {
	return i.ObjectId
}

func (i FileInfo) ToLimitReached() FileInfo {
	i.State = FileStateLimited
	return i
}

func (i FileInfo) ToUploading() FileInfo {
	i.State = FileStateUploading
	return i
}

func (i FileInfo) ToPendingDeletion() FileInfo {
	i.State = FileStatePendingDeletion
	return i
}

func (i FileInfo) ToDone() FileInfo {
	i.State = FileStateDone
	return i
}

func (i FileInfo) ToDeleted() FileInfo {
	i.State = FileStateDeleted
	return i
}

func (s *fileSync) processFilePendingUpload(ctx context.Context, it FileInfo) (FileInfo, error) {
	blocksAvailability, err := s.checkBlocksAvailability(ctx, it.ObjectId, it.SpaceId, it.FileId)
	if err != nil {
		return it, fmt.Errorf("check blocks availability: %w", err)
	}

	it.BytesToUpload = blocksAvailability.bytesToUpload
	it.CidsToUpload = blocksAvailability.cidsToUpload

	spaceLimits, err := s.limitManager.getSpace(ctx, it.SpaceId)
	if err != nil {
		return it, fmt.Errorf("get space limits: %w", err)
	}

	allocateErr := spaceLimits.allocateFile(ctx, it.Key(), blocksAvailability.totalBytesToUpload())
	// TODO De-allocate if error is occurred
	if allocateErr != nil {
		it = it.ToLimitReached()

		err = s.handleLimitReached(ctx, it)
		if err != nil {
			return it, fmt.Errorf("handle limit reached: %w", err)
		}
		return it, nil
	}

	if it.ObjectId != "" {

		err = s.updateStatus(it, filesyncstatus.Syncing)
		if isObjectDeletedError(err) {
			return it.ToPendingDeletion(), nil
		}
	}

	var totalBytesUploaded int
	err = s.walkFileBlocks(ctx, it.SpaceId, it.FileId, it.Variants, func(fileBlocks []blocks.Block) error {
		bytesToUpload, err := s.uploadOrBindBlocks(ctx, it, fileBlocks, blocksAvailability.cidsToUpload)
		if err != nil {
			return fmt.Errorf("select blocks to upload: %w", err)
		}
		totalBytesUploaded += bytesToUpload
		return nil
	})

	if err != nil {
		if isNodeLimitReachedError(err) {
			it = it.ToLimitReached()

			err = s.handleLimitReached(ctx, it)
			if err != nil {
				return it, fmt.Errorf("handle limit reached: %w", err)
			}
			return it, nil
		}
		return it, fmt.Errorf("walk file blocks: %w", err)
	}

	return it.ToUploading(), nil
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
		it.ToDone()
		return it, nil
	}

	return it, nil
}

func (s *fileSync) processFileLimited(fi FileInfo) (FileInfo, error) {
	// TODO Near the same as pending upload
	return fi, nil
}

func (s *fileSync) processFilePendingDeletion(ctx context.Context, fi FileInfo) (FileInfo, error) {
	log.Info("removing file", zap.String("fileId", fi.FileId.String()))
	err := s.rpcStore.DeleteFiles(ctx, fi.SpaceId, fi.FileId)
	if err != nil {
		return fi, err
	}
	log.Warn("file deleted", zap.String("fileId", fi.FileId.String()))

	return fi.ToDeleted(), nil
}

type ProcessAction int

const (
	ProcessActionNone = ProcessAction(iota)
	ProcessActionUpdate
	ProcessActionDelete
)

type filesRepository struct {
	lock  sync.Mutex
	files map[string]FileInfo
}

func newFilesRepository() *filesRepository {
	return &filesRepository{
		files: make(map[string]FileInfo),
	}
}

func (r *filesRepository) get(key string) (FileInfo, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	v, ok := r.files[key]
	return v, ok
}

func (r *filesRepository) find(pred func(file FileInfo) bool) (FileInfo, bool) {
	r.lock.Lock()
	defer r.lock.Unlock()

	for _, file := range r.files {
		if pred(file) {
			return file, true
		}
	}
	return FileInfo{}, false
}

func (r *filesRepository) put(key string, file FileInfo) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.files[key] = file
}

func (r *filesRepository) delete(key string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	delete(r.files, key)
}

type stateProcessor struct {
	filesRepository *filesRepository
	lock            sync.Mutex
	processing      map[string]*sync.Mutex
}

func newStateProcessor(repo *filesRepository) *stateProcessor {
	return &stateProcessor{
		filesRepository: repo,
		processing:      make(map[string]*sync.Mutex),
	}
}

func (q *stateProcessor) isProcessing(key string) bool {
	_, ok := q.processing[key]
	return ok
}

func (q *stateProcessor) process(key string, proc func(exists bool, info FileInfo) (ProcessAction, FileInfo, error)) {
	q.lock.Lock()
	procLock, ok := q.processing[key]
	if !ok {
		procLock = &sync.Mutex{}
		q.processing[key] = procLock
		procLock.Lock()
	}
	q.lock.Unlock()

	if ok {
		procLock.Lock()
	}
	defer procLock.Unlock()

	q.lock.Lock()
	fi, exists := q.filesRepository.get(key)
	q.lock.Unlock()

	// Critical section

	action, next, err := proc(exists, fi)
	if err != nil {
		log.Error("process item", zap.String("key", key), zap.Error(err))
	}

	q.lock.Lock()
	switch action {
	case ProcessActionNone:
	case ProcessActionUpdate:
		q.filesRepository.put(key, next)
	case ProcessActionDelete:
		q.filesRepository.delete(key)
	}
	delete(q.processing, key)
	q.lock.Unlock()
}

func (s *fileSync) runUploader(ctx context.Context) {
	ticker := time.NewTicker(time.Millisecond * 500)
	defer ticker.Stop()

	// TODO Decide what to do with items with Uploading status. There are at least two variants:
	// 1. Just try to upload Cids from CidsToUpload -> maybe more cleaner approach, because state machine will be described more thoughtfully.
	// 2. Add to pending upload queue

	for {
		select {
		case <-ctx.Done():
			return
		default:
			s.processNextPendingUploadItem(ctx)
		}
	}
}

func (s *fileSync) processNextPendingUploadItem(ctx context.Context) error {
	item, err := s.queue.GetNextScheduled(filequeue.GetNextScheduledRequest[FileInfo]{
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
			return info.State == FileStatePendingUpload
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

func (s *fileSync) process(id string, proc func(exists bool, info FileInfo) (ProcessAction, FileInfo, error)) error {
	item, err := s.queue.GetById(id)
	if err != nil && !errors.Is(err, filequeue.ErrNotFound) {
		return fmt.Errorf("get item: %w", err)
	}
	exists := !errors.Is(err, filequeue.ErrNotFound)

	act, next, err := proc(exists, item)
	if err != nil {
		return errors.Join(s.queue.Release(item), fmt.Errorf("process item: %w", err))
	}

	_ = act

	return s.queue.Release(next)
}

// func (s *fileSync) runUploadingProcessor(ctx context.Context) {
//
// 	ticker := time.NewTicker(time.Millisecond * 500)
// 	defer ticker.Stop()
//
// 	s.processNextPendingUploadItem(ctx)
// 	for {
// 		select {
// 		case <-ticker.C:
// 			s.processNextPendingUploadItem(ctx)
// 		case <-ctx.Done():
// 			return
// 		}
// 	}
// }
//
// func (s *fileSync) processNextUploadingItem(ctx context.Context) {
// 	next, ok := s.filesRepository.find(func(file FileInfo) bool {
// 		return file.State == FileStateUploading && time.Since(file.HandledAt) > time.Minute
// 	})
//
// 	if ok {
// 		s.stateProcessor.process(next.Key(), func(exists bool, info FileInfo) (ProcessAction, FileInfo, error) {
// 			if info.State == FileStatePendingUpload {
// 				next, err := s.processFilePendingUpload(ctx, info)
// 				return ProcessActionUpdate, next, err
// 			}
// 			return ProcessActionNone, info, nil
// 		})
// 	}
// }
