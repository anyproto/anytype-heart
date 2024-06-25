package filesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/net/peer"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

func (s *fileSync) AddFile(fileObjectId string, fileId domain.FullFileId, uploadedByUser, imported bool) (err error) {
	it := &QueueItem{
		ObjectId:    fileObjectId,
		SpaceId:     fileId.SpaceId,
		FileId:      fileId.FileId,
		AddedByUser: uploadedByUser,
		Imported:    imported,
		Timestamp:   time.Now().UnixMilli(),
	}
	err = it.Validate()
	if err != nil {
		return fmt.Errorf("validate queue item: %w", err)
	}

	if !s.fileIsInAnyQueue(it.Key()) {
		return s.uploadingQueue.Add(it)
	}
	return nil
}

func (s *fileSync) fileIsInAnyQueue(itemKey string) bool {
	return s.uploadingQueue.Has(itemKey) ||
		s.retryUploadingQueue.Has(itemKey) ||
		s.deletionQueue.Has(itemKey) ||
		s.retryDeletionQueue.Has(itemKey)
}

func (s *fileSync) SendImportEvents() {
	s.importEventsMutex.Lock()
	defer s.importEventsMutex.Unlock()
	for _, event := range s.importEvents {
		s.eventSender.Broadcast(event)
	}
}

func (s *fileSync) ClearImportEvents() {
	s.importEventsMutex.Lock()
	defer s.importEventsMutex.Unlock()
	s.importEvents = nil
}

// handleLimitReachedError checks if the error is limit reached error and sends event if needed
// Returns true if limit reached error occurred
func (s *fileSync) handleLimitReachedError(err error, it *QueueItem) *errLimitReached {
	if err == nil {
		return nil
	}
	var limitReachedErr *errLimitReached
	if errors.As(err, &limitReachedErr) {
		setErr := s.isLimitReachedErrorLogged.Set(it.ObjectId, true)
		if setErr != nil {
			log.Error("set limit reached error logged", zap.String("objectId", it.ObjectId), zap.Error(setErr))
		}

		s.runOnLimitedHook(it.ObjectId, it.FullFileId())

		if it.AddedByUser && !it.Imported {
			s.sendLimitReachedEvent(it.SpaceId)
		}
		if it.Imported {
			s.addImportEvent(it.SpaceId)
		}
		return limitReachedErr
	}
	return nil
}

func (s *fileSync) uploadingHandler(ctx context.Context, it *QueueItem) (persistentqueue.Action, error) {
	spaceId, fileId := it.SpaceId, it.FileId
	err := s.uploadFile(ctx, spaceId, fileId, it.ObjectId)
	if isObjectDeletedError(err) {
		return persistentqueue.ActionDone, s.DeleteFile(it.ObjectId, it.FullFileId())
	}
	if err != nil {
		if limitErr := s.handleLimitReachedError(err, it); limitErr != nil {
			log.Warn("upload limit has been reached",
				zap.String("fileId", fileId.String()),
				zap.String("objectId", it.ObjectId),
				zap.Int("fileSize", limitErr.fileSize),
				zap.Int("accountLimit", limitErr.accountLimit),
				zap.Int("totalBytesUsage", limitErr.totalBytesUsage),
			)
		} else {
			log.Error("uploading file error",
				zap.String("fileId", fileId.String()), zap.Error(err),
				zap.String("objectId", it.ObjectId),
			)
		}

		return s.addToRetryUploadingQueue(it), nil
	}

	err = s.runOnUploadedHook(it.ObjectId, it.FullFileId())
	if isObjectDeletedError(err) {
		return persistentqueue.ActionDone, s.DeleteFile(it.ObjectId, it.FullFileId())
	}
	if err != nil {
		return s.addToRetryUploadingQueue(it), err
	}
	s.updateSpaceUsageInformation(spaceId)

	return persistentqueue.ActionDone, s.removeFromUploadingQueues(it.ObjectId)
}

func (s *fileSync) addToRetryUploadingQueue(it *QueueItem) persistentqueue.Action {
	err := s.retryUploadingQueue.Add(it)
	if err != nil {
		log.Error("can't add upload task to retrying queue", zap.String("fileId", it.FileId.String()), zap.Error(err))
		return persistentqueue.ActionRetry
	}
	return persistentqueue.ActionDone
}

func (s *fileSync) retryingHandler(ctx context.Context, it *QueueItem) (persistentqueue.Action, error) {
	spaceId, fileId := it.SpaceId, it.FileId
	err := s.uploadFile(ctx, spaceId, fileId, it.ObjectId)
	if isObjectDeletedError(err) {
		return persistentqueue.ActionDone, s.removeFromUploadingQueues(it.ObjectId)
	}
	if err != nil {
		limitErr := s.handleLimitReachedError(err, it)
		var limitErrorIsLogged bool
		if limitErr != nil {
			var hasErr error
			limitErrorIsLogged, hasErr = s.isLimitReachedErrorLogged.Has(it.ObjectId)
			if hasErr != nil {
				log.Error("check if limit reached error is logged", zap.String("objectId", it.ObjectId), zap.Error(hasErr))
			}
		}
		if limitErr == nil || !limitErrorIsLogged {
			log.Error("retry uploading file error",
				zap.String("fileId", fileId.String()), zap.Error(err),
				zap.String("objectId", it.ObjectId),
			)
		}

		return persistentqueue.ActionRetry, nil
	}

	err = s.runOnUploadedHook(it.ObjectId, it.FullFileId())
	if isObjectDeletedError(err) {
		return persistentqueue.ActionDone, s.DeleteFile(it.ObjectId, it.FullFileId())
	}
	if err != nil {
		return persistentqueue.ActionRetry, err
	}
	s.updateSpaceUsageInformation(spaceId)

	return persistentqueue.ActionDone, s.removeFromUploadingQueues(it.ObjectId)
}

func (s *fileSync) removeFromUploadingQueues(objectId string) error {
	if objectId == "" {
		return nil
	}
	err := s.uploadingQueue.Remove(objectId)
	if err != nil {
		return fmt.Errorf("remove upload task: %w", err)
	}
	err = s.retryUploadingQueue.Remove(objectId)
	if err != nil {
		return fmt.Errorf("remove upload task from retrying queue: %w", err)
	}
	return nil
}

// UploadSynchronously is used only for invites
func (s *fileSync) UploadSynchronously(ctx context.Context, spaceId string, fileId domain.FileId) error {
	// TODO After we migrate to storing invites as file objects in tech space, we should update their sync status
	//  via OnUploadStarted and OnUploaded callbacks
	err := s.uploadFile(ctx, spaceId, fileId, "")
	if err != nil {
		return err
	}
	s.updateSpaceUsageInformation(spaceId)
	return nil
}

func (s *fileSync) runOnUploadedHook(fileObjectId string, fileId domain.FullFileId) error {
	var errs error
	for _, hook := range s.onUploaded {
		err := hook(fileObjectId, fileId)
		if err != nil {
			if !isObjectDeletedError(err) {
				log.Warn("on upload callback failed",
					zap.String("spaceId", fileId.SpaceId),
					zap.String("fileObjectId", fileObjectId),
					zap.String("fileId", fileId.FileId.String()),
					zap.Error(err))
			}
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

func (s *fileSync) runOnUploadStartedHook(fileObjectId string, fileId domain.FullFileId) error {
	if s.onUploadStarted != nil {
		err := s.onUploadStarted(fileObjectId, fileId)
		if err != nil {
			if !isObjectDeletedError(err) {
				log.Warn("on upload started callback failed",
					zap.String("spaceId", fileId.SpaceId),
					zap.String("fileObjectId", fileObjectId),
					zap.String("fileId", fileId.FileId.String()),
					zap.Error(err))
			}
			return err
		}
	}
	return nil
}

func (s *fileSync) runOnLimitedHook(fileObjectId string, fileId domain.FullFileId) {
	if s.onLimited != nil {
		err := s.onLimited(fileObjectId, fileId)
		if err != nil {
			if !isObjectDeletedError(err) {
				log.Warn("on limited callback failed",
					zap.String("spaceId", fileId.SpaceId),
					zap.String("fileObjectId", fileObjectId),
					zap.String("fileId", fileId.FileId.String()),
					zap.Error(err))
			}
		}
	}
}

type errLimitReached struct {
	fileSize        int
	accountLimit    int
	totalBytesUsage int
}

func (e *errLimitReached) Error() string {
	return "file upload limit has been reached"
}

func (s *fileSync) uploadFile(ctx context.Context, spaceID string, fileId domain.FileId, objectId string) error {
	ctx = filestorage.ContextWithDoNotCache(ctx)
	log.Debug("uploading file", zap.String("fileId", fileId.String()))

	blocksAvailability, err := s.blocksAvailabilityCache.Get(fileId.String())
	if err != nil {
		// Ignore error from cache and calculate blocks availability
		blocksAvailability, err = s.checkBlocksAvailability(ctx, spaceID, fileId)
		if err != nil {
			return fmt.Errorf("check blocks availability: %w", err)
		}
		err = s.blocksAvailabilityCache.Set(fileId.String(), blocksAvailability)
		if err != nil {
			log.Error("cache blocks availability", zap.String("fileId", fileId.String()), zap.Error(err))
		}
	}

	stat, err := s.getAndUpdateSpaceStat(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("get space stat: %w", err)
	}

	bytesLeft := stat.AccountBytesLimit - stat.TotalBytesUsage
	if blocksAvailability.bytesToUpload > bytesLeft {
		// Unbind file just in case
		err := s.rpcStore.DeleteFiles(ctx, spaceID, fileId)
		if err != nil {
			log.Error("calculate limits: unbind off-limit file", zap.String("fileId", fileId.String()), zap.Error(err))
		}
		return &errLimitReached{
			fileSize:        blocksAvailability.bytesToUpload,
			accountLimit:    stat.AccountBytesLimit,
			totalBytesUsage: stat.TotalBytesUsage,
		}
	}
	if objectId != "" {
		err = s.runOnUploadStartedHook(objectId, domain.FullFileId{FileId: fileId, SpaceId: spaceID})
		if isObjectDeletedError(err) {
			return err
		}
	}
	var totalBytesUploaded int
	err = s.walkFileBlocks(ctx, spaceID, fileId, func(fileBlocks []blocks.Block) error {
		bytesToUpload, err := s.uploadOrBindBlocks(ctx, spaceID, fileId, fileBlocks, blocksAvailability.cidsToUpload)
		if err != nil {
			return fmt.Errorf("select blocks to upload: %w", err)
		}
		totalBytesUploaded += bytesToUpload
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), fileprotoerr.ErrSpaceLimitExceeded.Error()) {
			// Unbind partially uploaded file
			err := s.rpcStore.DeleteFiles(ctx, spaceID, fileId)
			if err != nil {
				log.Error("upload: unbind off-limit file", zap.String("fileId", fileId.String()), zap.Error(err))
			}
			return &errLimitReached{
				fileSize:        blocksAvailability.bytesToUpload,
				accountLimit:    stat.AccountBytesLimit,
				totalBytesUsage: stat.TotalBytesUsage,
			}
		}
		return fmt.Errorf("walk file blocks: %w", err)
	}

	err = s.blocksAvailabilityCache.Delete(fileId.String())
	if err != nil {
		log.Warn("delete blocks availability cache entry", zap.String("fileId", fileId.String()), zap.Error(err))
	}
	err = s.isLimitReachedErrorLogged.Delete(fileId.String())
	if err != nil {
		log.Warn("delete limit reached error logged", zap.String("fileId", fileId.String()), zap.Error(err))
	}
	log.Warn("done upload", zap.String("fileId", fileId.String()), zap.Int("bytesToUpload", blocksAvailability.bytesToUpload), zap.Int("bytesUploaded", totalBytesUploaded))

	return nil
}

func (s *fileSync) sendLimitReachedEvent(spaceID string) {
	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfFileLimitReached{
					FileLimitReached: &pb.EventFileLimitReached{
						SpaceId: spaceID,
					},
				},
			},
		},
	})
}

func (s *fileSync) addImportEvent(spaceID string) {
	s.importEventsMutex.Lock()
	defer s.importEventsMutex.Unlock()
	s.importEvents = append(s.importEvents, &pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfFileLimitReached{
					FileLimitReached: &pb.EventFileLimitReached{
						SpaceId: spaceID,
					},
				},
			},
		},
	})
}

type blocksAvailabilityResponse struct {
	bytesToUpload int
	cidsToUpload  map[cid.Cid]struct{}
}

type blocksAvailabilityResponseJson struct {
	BytesToUpload int
	CidsToUpload  []string
}

var _ json.Marshaler = &blocksAvailabilityResponse{}

func (r *blocksAvailabilityResponse) MarshalJSON() ([]byte, error) {
	wrapper := blocksAvailabilityResponseJson{
		BytesToUpload: r.bytesToUpload,
	}
	for c := range r.cidsToUpload {
		wrapper.CidsToUpload = append(wrapper.CidsToUpload, c.String())
	}
	return json.Marshal(wrapper)
}

func (r *blocksAvailabilityResponse) UnmarshalJSON(data []byte) error {
	var wrapper blocksAvailabilityResponseJson
	err := json.Unmarshal(data, &wrapper)
	if err != nil {
		return err
	}
	r.bytesToUpload = wrapper.BytesToUpload
	r.cidsToUpload = map[cid.Cid]struct{}{}
	for _, rawCid := range wrapper.CidsToUpload {
		cid, err := cid.Parse(rawCid)
		if err != nil {
			return err
		}
		r.cidsToUpload[cid] = struct{}{}
	}
	return nil
}

func (s *fileSync) checkBlocksAvailability(ctx context.Context, spaceId string, fileId domain.FileId) (*blocksAvailabilityResponse, error) {
	response := blocksAvailabilityResponse{
		cidsToUpload: map[cid.Cid]struct{}{},
	}
	err := s.walkFileBlocks(ctx, spaceId, fileId, func(fileBlocks []blocks.Block) error {
		fileCids := lo.Map(fileBlocks, func(b blocks.Block, _ int) cid.Cid {
			return b.Cid()
		})
		availabilities, err := s.rpcStore.CheckAvailability(ctx, spaceId, fileCids)
		if err != nil {
			return fmt.Errorf("check availability: %w", err)
		}
		for _, availability := range availabilities {
			blockCid, err := cid.Cast(availability.Cid)
			if err != nil {
				return fmt.Errorf("cast cid: %w", err)
			}

			if availability.Status == fileproto.AvailabilityStatus_NotExists {
				b, ok := lo.Find(fileBlocks, func(b blocks.Block) bool {
					return b.Cid() == blockCid
				})
				if !ok {
					return fmt.Errorf("block %s not found", blockCid)
				}
				response.bytesToUpload += len(b.RawData())
				response.cidsToUpload[blockCid] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk DAG: %w", err)
	}
	return &response, nil
}

func (s *fileSync) uploadOrBindBlocks(ctx context.Context, spaceId string, fileId domain.FileId, fileBlocks []blocks.Block, needToUpload map[cid.Cid]struct{}) (int, error) {
	var (
		bytesToUpload  int
		blocksToUpload []blocks.Block
		cidsToBind     []cid.Cid
	)

	for _, b := range fileBlocks {
		blockCid := b.Cid()
		if _, ok := needToUpload[blockCid]; ok {
			blocksToUpload = append(blocksToUpload, b)
			bytesToUpload += len(b.RawData())
		} else {
			cidsToBind = append(cidsToBind, blockCid)
		}
	}

	if len(cidsToBind) > 0 {
		if bindErr := s.rpcStore.BindCids(ctx, spaceId, fileId, cidsToBind); bindErr != nil {
			return 0, fmt.Errorf("bind cids: %w", bindErr)
		}
	}

	if len(blocksToUpload) > 0 {
		err := s.rpcStore.AddToFile(ctx, spaceId, fileId, blocksToUpload)
		if err != nil {
			return 0, fmt.Errorf("add to file: %w", err)
		}
	}
	return bytesToUpload, nil
}

func isObjectDeletedError(err error) bool {
	return errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) || errors.Is(err, peer.ErrPeerIdNotFoundInContext) || errors.Is(err, domain.ErrObjectIsDeleted)
}
