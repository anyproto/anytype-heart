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
	format "github.com/ipfs/go-ipld-format"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

type AddFileRequest struct {
	FileObjectId   string
	FileId         domain.FullFileId
	UploadedByUser bool
	Imported       bool
	VariantId      domain.FileId

	// PrioritizeVariantId tells uploader to upload specific branch of file tree
	PrioritizeVariantId domain.FileId
	// Score affects priority, files with higher score are uploaded first
	Score int
}

func (req AddFileRequest) ToQueueItem(addedTime time.Time) (*QueueItem, error) {
	it := &QueueItem{
		ObjectId:    req.FileObjectId,
		SpaceId:     req.FileId.SpaceId,
		FileId:      req.FileId.FileId,
		AddedByUser: req.UploadedByUser,
		Imported:    req.Imported,
		Timestamp:   float64(addedTime.UnixMilli()),
		VariantId:   req.PrioritizeVariantId,
		Score:       req.Score,
	}
	err := it.Validate()
	if err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	return it, nil
}

func (s *fileSync) AddFile(req AddFileRequest) (err error) {
	it, err := req.ToQueueItem(time.Now())
	if err != nil {
		return err
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
		setErr := s.isLimitReachedErrorLogged.Set(context.Background(), it.ObjectId, true)
		if setErr != nil {
			log.Error("set limit reached error logged", zap.String("objectId", it.ObjectId), zap.Error(setErr))
		}

		var bytesLeftPercentage float64
		if limitReachedErr.accountLimit != 0 {
			bytesLeftPercentage = float64(limitReachedErr.accountLimit-limitReachedErr.totalBytesUsage) / float64(limitReachedErr.accountLimit)
		}
		s.runOnLimitedHook(it.ObjectId, it.FullFileId(), bytesLeftPercentage)

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
	err := s.uploadFile(ctx, it)
	if errors.Is(err, context.Canceled) {
		return persistentqueue.ActionRetry, nil
	}
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

	// Mark as uploaded only if the root of the file tree is uploaded. It works because if the root is uploaded, all its descendants are uploaded too
	if it.VariantId == "" {
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

	s.updateSpaceUsageInformation(spaceId)
	return persistentqueue.ActionDone, nil
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
	err := s.uploadFile(ctx, it)
	if errors.Is(err, context.Canceled) {
		return persistentqueue.ActionRetry, nil
	}
	if isObjectDeletedError(err) {
		return persistentqueue.ActionDone, s.removeFromUploadingQueues(it.ObjectId)
	}
	if err != nil {
		limitErr := s.handleLimitReachedError(err, it)
		var limitErrorIsLogged bool
		if limitErr != nil {
			var hasErr error
			limitErrorIsLogged, hasErr = s.isLimitReachedErrorLogged.Has(ctx, it.ObjectId)
			if hasErr != nil {
				log.Error("check if limit reached error is logged", zap.String("objectId", it.ObjectId), zap.Error(hasErr))
			}
		}
		if limitErr == nil || !limitErrorIsLogged {
			if !format.IsNotFound(err) && !strings.Contains(err.Error(), "failed to fetch all nodes") {
				log.Error("retry uploading file error",
					zap.String("fileId", fileId.String()), zap.Error(err),
					zap.String("objectId", it.ObjectId),
				)
			}
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
	err := s.uploadingQueue.RemoveBy(func(key string) bool {
		return strings.HasPrefix(key, objectId)
	})
	if err != nil {
		return fmt.Errorf("remove upload task: %w", err)
	}
	err = s.retryUploadingQueue.RemoveBy(func(key string) bool {
		return strings.HasPrefix(key, objectId)
	})
	if err != nil {
		return fmt.Errorf("remove upload task from retrying queue: %w", err)
	}
	return nil
}

// UploadSynchronously is used only for invites
func (s *fileSync) UploadSynchronously(ctx context.Context, spaceId string, fileId domain.FileId) error {
	// TODO After we migrate to storing invites as file objects in tech space, we should update their sync status
	//  via OnUploadStarted and OnUploaded callbacks
	err := s.uploadFile(ctx, &QueueItem{SpaceId: spaceId, FileId: fileId})
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

func (s *fileSync) runOnLimitedHook(fileObjectId string, fileId domain.FullFileId, bytesLeftPercentage float64) {
	if s.onLimited != nil {
		err := s.onLimited(fileObjectId, fileId, bytesLeftPercentage)
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

func (s *fileSync) uploadFile(ctx context.Context, it *QueueItem) error {
	ctx = filestorage.ContextWithDoNotCache(ctx)
	log.Debug("uploading file", zap.String("fileId", it.FileId.String()))

	branchToUpload := it.FileId
	if it.VariantId != "" {
		branchToUpload = it.VariantId
	}

	blocksAvailability, err := s.blocksAvailabilityCache.Get(ctx, branchToUpload.String())
	if err != nil || blocksAvailability.totalBytesToUpload() == 0 {
		// Ignore error from cache and calculate blocks availability
		blocksAvailability, err = s.checkBlocksAvailability(ctx, it.SpaceId, branchToUpload)
		if err != nil {
			return fmt.Errorf("check blocks availability: %w", err)
		}
		err = s.blocksAvailabilityCache.Set(ctx, branchToUpload.String(), blocksAvailability)
		if err != nil {
			log.Error("cache blocks availability", zap.String("fileId", it.FileId.String()), zap.Error(err))
		}
	}

	stat, err := s.getAndUpdateSpaceStat(ctx, it.SpaceId)
	if err != nil {
		return fmt.Errorf("get space stat: %w", err)
	}

	bytesLeft := stat.AccountBytesLimit - stat.TotalBytesUsage
	if blocksAvailability.totalBytesToUpload() > bytesLeft {
		// Unbind file just in case
		err := s.rpcStore.DeleteFiles(ctx, it.SpaceId, it.FileId)
		if err != nil {
			log.Error("calculate limits: unbind off-limit file", zap.String("fileId", it.FileId.String()), zap.Error(err))
		}
		return &errLimitReached{
			fileSize:        blocksAvailability.totalBytesToUpload(),
			accountLimit:    stat.AccountBytesLimit,
			totalBytesUsage: stat.TotalBytesUsage,
		}
	}
	if it.ObjectId != "" {
		err = s.runOnUploadStartedHook(it.ObjectId, domain.FullFileId{FileId: it.FileId, SpaceId: it.SpaceId})
		if isObjectDeletedError(err) {
			return err
		}
	}
	var totalBytesUploaded int

	err = s.walkFileBlocks(ctx, it.SpaceId, branchToUpload, func(fileBlocks []blocks.Block) error {
		bytesToUpload, err := s.uploadOrBindBlocks(ctx, it.SpaceId, it.FileId, fileBlocks, blocksAvailability.cidsToUpload)
		if err != nil {
			return fmt.Errorf("select blocks to upload: %w", err)
		}
		totalBytesUploaded += bytesToUpload
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), fileprotoerr.ErrSpaceLimitExceeded.Error()) {
			// Unbind partially uploaded file
			err := s.rpcStore.DeleteFiles(ctx, it.SpaceId, it.FileId)
			if err != nil {
				log.Error("upload: unbind off-limit file", zap.String("fileId", it.FileId.String()), zap.Error(err))
			}
			return &errLimitReached{
				fileSize:        blocksAvailability.totalBytesToUpload(),
				accountLimit:    stat.AccountBytesLimit,
				totalBytesUsage: stat.TotalBytesUsage,
			}
		}

		return fmt.Errorf("walk file blocks: %w", err)
	}

	err = s.blocksAvailabilityCache.Delete(ctx, branchToUpload.String())
	if err != nil {
		log.Warn("delete blocks availability cache entry", zap.String("fileId", it.FileId.String()), zap.Error(err))
	}
	err = s.isLimitReachedErrorLogged.Delete(ctx, it.FileId.String())
	if err != nil {
		log.Warn("delete limit reached error logged", zap.String("fileId", it.FileId.String()), zap.Error(err))
	}

	return nil
}

func (s *fileSync) sendLimitReachedEvent(spaceID string) {
	s.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfFileLimitReached{
		FileLimitReached: &pb.EventFileLimitReached{
			SpaceId: spaceID,
		},
	}))
}

func (s *fileSync) addImportEvent(spaceID string) {
	s.importEventsMutex.Lock()
	defer s.importEventsMutex.Unlock()
	s.importEvents = append(s.importEvents, event.NewEventSingleMessage("", &pb.EventMessageValueOfFileLimitReached{
		FileLimitReached: &pb.EventFileLimitReached{
			SpaceId: spaceID,
		}}))
}

type blocksAvailabilityResponse struct {
	bytesToUpload int
	bytesToBind   int
	cidsToUpload  map[cid.Cid]struct{}
}

func (r *blocksAvailabilityResponse) totalBytesToUpload() int {
	return r.bytesToUpload + r.bytesToBind
}

type blocksAvailabilityResponseJson struct {
	BytesToUpload int
	BytesToBind   int
	CidsToUpload  []string
}

var _ json.Marshaler = &blocksAvailabilityResponse{}

func (r *blocksAvailabilityResponse) MarshalJSON() ([]byte, error) {
	wrapper := blocksAvailabilityResponseJson{
		BytesToUpload: r.bytesToUpload,
		BytesToBind:   r.bytesToBind,
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
	r.bytesToBind = wrapper.BytesToBind
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

			getBlock := func() (blocks.Block, error) {
				b, ok := lo.Find(fileBlocks, func(b blocks.Block) bool {
					return b.Cid() == blockCid
				})
				if !ok {
					return nil, fmt.Errorf("block %s not found", blockCid)
				}
				return b, nil
			}

			if availability.Status == fileproto.AvailabilityStatus_NotExists {
				b, err := getBlock()
				if err != nil {
					return err
				}
				response.bytesToUpload += len(b.RawData())
				response.cidsToUpload[blockCid] = struct{}{}
			} else if availability.Status == fileproto.AvailabilityStatus_Exists {
				// Block exists in node, but not in user's space
				b, err := getBlock()
				if err != nil {
					return err
				}
				response.bytesToBind += len(b.RawData())
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
