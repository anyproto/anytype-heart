package filesync

import (
	"context"
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
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
)

type AddFileRequest struct {
	FileObjectId   string
	FileId         domain.FullFileId
	UploadedByUser bool
	Imported       bool

	Variants []domain.FileId
}

func (s *fileSync) AddFile(req AddFileRequest) error {
	if s.cfg.IsLocalOnlyMode() {
		return nil
	}
	if !req.FileId.Valid() {
		return fmt.Errorf("invalid file id: %q", req.FileId)
	}

	return s.process(req.FileObjectId, func(exists bool, info FileInfo) (FileInfo, bool, error) {
		if exists && info.State.IsUploadingState() {
			return info, false, nil
		}
		info = FileInfo{
			FileId:       req.FileId.FileId,
			SpaceId:      req.FileId.SpaceId,
			ObjectId:     req.FileObjectId,
			State:        FileStatePendingUpload,
			ScheduledAt:  time.Now(),
			Variants:     req.Variants,
			AddedByUser:  req.UploadedByUser,
			Imported:     req.Imported,
			CidsToUpload: map[cid.Cid]struct{}{},
			CidsToBind:   map[cid.Cid]struct{}{},
		}
		return info, true, nil
	})
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

func (s *fileSync) resetUploadingStatus(ctx context.Context) error {
	item, err := s.queue.GetNext(ctx, filequeue.GetNextRequest[FileInfo]{
		Subscribe:   false,
		StoreFilter: filterByState(FileStateUploading),
		Filter: func(info FileInfo) bool {
			return info.State == FileStateUploading
		},
	})
	if err != nil {
		return fmt.Errorf("get next scheduled item: %w", err)
	}

	item.State = FileStatePendingUpload
	item.ScheduledAt = time.Now()

	releaseErr := s.queue.ReleaseAndUpdate(item.ObjectId, item)

	return errors.Join(releaseErr, err)
}

func (s *fileSync) runUploader(ctx context.Context) {

	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := s.processNextPendingUploadItem(ctx, FileStatePendingUpload)
			if err != nil && !errors.Is(err, filequeue.ErrClosed) {
				log.Error("process next pending upload item", zap.Error(err))
			}
		}
	}
}

func (s *fileSync) processNextPendingUploadItem(ctx context.Context, state FileState) error {
	item, err := s.queue.GetNextScheduled(ctx, filequeue.GetNextScheduledRequest[FileInfo]{
		Subscribe:   true,
		StoreFilter: filterByState(state),
		StoreOrder:  orderByScheduledAt(),
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

	releaseErr := s.queue.ReleaseAndUpdate(item.ObjectId, next)

	return errors.Join(releaseErr, err)
}

func (s *fileSync) processFilePendingUpload(ctx context.Context, it FileInfo) (FileInfo, error) {
	blocksAvailability, err := s.checkBlocksAvailability(ctx, it)
	if err != nil {
		it = it.Reschedule()
		return it, fmt.Errorf("check blocks availability: %w", err)
	}

	it.BytesToUploadOrBind = blocksAvailability.bytesToUploadOrBind
	it.CidsToBind = blocksAvailability.cidsToBind
	it.CidsToUpload = blocksAvailability.cidsToUpload

	spaceLimits, err := s.limitManager.getSpace(ctx, it.SpaceId)
	if err != nil {
		it = it.Reschedule()
		return it, fmt.Errorf("get space limits: %w", err)
	}

	allocateErr := spaceLimits.allocateFile(ctx, it.Key(), blocksAvailability.bytesToUploadOrBind)
	if allocateErr != nil {
		it.State = FileStateLimited
		it = it.Reschedule()

		err = s.handleLimitReached(ctx, it)
		if err != nil {
			return it, fmt.Errorf("handle limit reached: %w", err)
		}
		return it, nil
	}

	it, err = s.upload(ctx, it, blocksAvailability)
	if err != nil {
		spaceLimits.deallocateFile(it.Key())
		it = it.Reschedule()
		return it, err
	}
	return it, nil
}

func (s *fileSync) upload(ctx context.Context, it FileInfo, blocksAvailability *blocksAvailabilityResponse) (FileInfo, error) {
	if it.ObjectId != "" {
		err := s.updateStatus(it, filesyncstatus.Syncing)
		if isObjectDeletedError(err) {
			it.State = FileStatePendingDeletion
			return it, nil
		}
		if err != nil {
			return it, fmt.Errorf("update status: %w", err)
		}
	}

	var totalBytesToUpload int
	err := s.walkFileBlocks(ctx, it.SpaceId, it.FileId, it.Variants, func(fileBlocks []blocks.Block) error {
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

type errLimitReached struct {
	fileSize        int
	accountLimit    int
	totalBytesUsage int
}

func (e *errLimitReached) Error() string {
	return "file upload limit has been reached"
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
	bytesToUploadOrBind int
	cidsToBind          map[cid.Cid]struct{}
	cidsToUpload        map[cid.Cid]struct{}
}

func (s *fileSync) checkBlocksAvailability(ctx context.Context, info FileInfo) (*blocksAvailabilityResponse, error) {
	if len(info.CidsToBind) > 0 || len(info.CidsToUpload) > 0 {
		return &blocksAvailabilityResponse{
			bytesToUploadOrBind: info.BytesToUploadOrBind,
			cidsToBind:          info.CidsToBind,
			cidsToUpload:        info.CidsToUpload,
		}, nil
	}

	response := blocksAvailabilityResponse{
		cidsToBind:   map[cid.Cid]struct{}{},
		cidsToUpload: map[cid.Cid]struct{}{},
	}
	err := s.walkFileBlocks(ctx, info.SpaceId, info.FileId, nil, func(fileBlocks []blocks.Block) error {
		fileCids := lo.Map(fileBlocks, func(b blocks.Block, _ int) cid.Cid {
			return b.Cid()
		})
		availabilities, err := s.rpcStore.CheckAvailability(ctx, info.SpaceId, fileCids)
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
				response.bytesToUploadOrBind += len(b.RawData())
				response.cidsToUpload[blockCid] = struct{}{}
			} else if availability.Status == fileproto.AvailabilityStatus_Exists {
				// Block exists in node, but not in user's space
				b, err := getBlock()
				if err != nil {
					return err
				}
				response.cidsToBind[blockCid] = struct{}{}
				response.bytesToUploadOrBind += len(b.RawData())
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk DAG: %w", err)
	}
	return &response, nil
}

func (s *fileSync) uploadOrBindBlocks(ctx context.Context, fi FileInfo, fileBlocks []blocks.Block, needToBind map[cid.Cid]struct{}) (int, error) {
	var (
		bytesToUpload  int
		blocksToUpload []blocks.Block
		cidsToBind     []cid.Cid
	)

	for _, b := range fileBlocks {
		blockCid := b.Cid()
		if _, ok := needToBind[blockCid]; ok {
			cidsToBind = append(cidsToBind, blockCid)
		} else {
			blocksToUpload = append(blocksToUpload, b)
			bytesToUpload += len(b.RawData())
		}
	}

	if len(cidsToBind) > 0 {
		if bindErr := s.rpcStore.BindCids(ctx, fi.SpaceId, fi.FileId, cidsToBind); bindErr != nil {
			return 0, fmt.Errorf("bind cids: %w", bindErr)
		}
	}

	if len(blocksToUpload) > 0 {
		err := s.requestsBatcher.addFile(fi.SpaceId, fi.FileId.String(), fi.ObjectId, blocksToUpload)
		if err != nil {
			return 0, fmt.Errorf("add to file: %w", err)
		}
	}
	return bytesToUpload, nil
}

func isObjectDeletedError(err error) bool {
	return errors.Is(err, spacestorage.ErrTreeStorageAlreadyDeleted) || errors.Is(err, peer.ErrPeerIdNotFoundInContext) || errors.Is(err, domain.ErrObjectIsDeleted)
}

func isNodeLimitReachedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), fileprotoerr.ErrSpaceLimitExceeded.Error())
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
