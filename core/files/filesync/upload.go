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

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
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

func (req AddFileRequest) ToQueueItem(addedTime time.Time) (*QueueItem, error) {
	it := &QueueItem{
		ObjectId:    req.FileObjectId,
		SpaceId:     req.FileId.SpaceId,
		FileId:      req.FileId.FileId,
		AddedByUser: req.UploadedByUser,
		Imported:    req.Imported,
		Timestamp:   float64(addedTime.UnixMilli()),
		Variants:    req.Variants,
	}
	err := it.Validate()
	if err != nil {
		return nil, fmt.Errorf("validate: %w", err)
	}
	return it, nil
}

func (s *fileSync) AddFile(req AddFileRequest) (err error) {
	return s.process(req.FileObjectId, func(_ bool, _ FileInfo) (ProcessAction, FileInfo, error) {
		info := FileInfo{
			FileId:        req.FileId.FileId,
			SpaceId:       req.FileId.SpaceId,
			ObjectId:      req.FileObjectId,
			State:         FileStatePendingUpload,
			ScheduledAt:   time.Now(),
			Variants:      req.Variants,
			AddedByUser:   req.UploadedByUser,
			Imported:      req.Imported,
			BytesToUpload: 0,
			CidsToUpload:  map[cid.Cid]struct{}{},
		}
		return ProcessActionUpdate, info, nil
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

// UploadSynchronously is used only for invites
func (s *fileSync) UploadSynchronously(ctx context.Context, spaceId string, fileId domain.FileId) error {
	return fmt.Errorf("TODO")
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

func (s *fileSync) checkBlocksAvailability(ctx context.Context, fileObjectId string, spaceId string, fileId domain.FileId) (*blocksAvailabilityResponse, error) {
	response := blocksAvailabilityResponse{
		cidsToUpload: map[cid.Cid]struct{}{},
	}
	err := s.walkFileBlocks(ctx, spaceId, fileId, nil, func(fileBlocks []blocks.Block) error {
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

func (s *fileSync) uploadOrBindBlocks(ctx context.Context, fi FileInfo, fileBlocks []blocks.Block, needToUpload map[cid.Cid]struct{}) (int, error) {
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
		if bindErr := s.rpcStore.BindCids(ctx, fi.SpaceId, fi.FileId, cidsToBind); bindErr != nil {
			return 0, fmt.Errorf("bind cids: %w", bindErr)
		}
		if len(blocksToUpload) == 0 {
			err := s.updateStatus(fi, filesyncstatus.Synced)
			if err != nil {
				return 0, fmt.Errorf("add to status update queue: %w", err)
			}
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
