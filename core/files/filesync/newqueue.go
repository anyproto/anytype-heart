package filesync

import (
	"context"
	"fmt"
	"sync"
	"time"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
)

type FileState int

const (
	FileStatePendingUpload FileState = iota
	FileStateUploading
	FileStateLimited
	FileStatePendingDeletion
	FileStateDone
)

type FileInfo struct {
	FileId      domain.FileId
	SpaceId     string
	ObjectId    string
	State       FileState
	AddedAt     time.Time
	HandledAt   time.Time
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
	i.HandledAt = time.Now()
	i.State = FileStateLimited
	return i
}

func (i FileInfo) ToUploading() FileInfo {
	i.HandledAt = time.Now()
	i.State = FileStateUploading
	return i
}

func (i FileInfo) ToPendingDeletion() FileInfo {
	i.HandledAt = time.Now()
	i.State = FileStatePendingDeletion
	return i
}

func (i FileInfo) ToDone() FileInfo {
	i.State = FileStateDone
	return i
}

func (s *fileSync) processFile(ctx context.Context, fi FileInfo) (FileInfo, error) {
	switch fi.State {
	case FileStatePendingUpload:
		return s.processFilePendingUpload(ctx, fi)
	case FileStateUploading:
		return s.processFileUploading(ctx, fi)
	case FileStateLimited:
		return s.processFileLimited(fi)
	case FileStatePendingDeletion:
		return s.processFilePendingDeletion(ctx, fi)
	default:
		return fi, fmt.Errorf("unknown state: %d", fi.State)
	}
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

	it.HandledAt = time.Now()
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

	return fi.ToDone(), nil
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
