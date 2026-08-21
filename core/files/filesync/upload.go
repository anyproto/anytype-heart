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
	"github.com/anyproto/any-sync/net"
	"github.com/anyproto/any-sync/net/peer"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	rpcstore2 "github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
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

	// Note: the file's blocks are not required to exist locally. A file object
	// may reference a fileId whose blocks live only on the file node — e.g.
	// import dedup reuses a fileId uploaded earlier from another space or
	// device. The uploader confirms such files by checking the node and
	// binding the cids it already has.
	return s.process(req.FileObjectId, func(exists bool, info FileInfo) (FileInfo, bool, error) {
		if exists && info.State.IsUploadingState() {
			return info, false, nil
		}
		// A parked missing-blocks item is woken for an immediate re-check: an
		// explicit AddFile means the file was re-added or its object was
		// opened, so its blocks may have just become available locally. The
		// retry counter is kept, so a still-broken file returns to its long
		// backoff after one attempt.
		var missingBlocksRetries int
		if exists {
			missingBlocksRetries = info.MissingBlocksRetries
		}
		info = FileInfo{
			FileId:               req.FileId.FileId,
			SpaceId:              req.FileId.SpaceId,
			ObjectId:             req.FileObjectId,
			State:                FileStatePendingUpload,
			ScheduledAt:          time.Now(),
			Variants:             req.Variants,
			AddedByUser:          req.UploadedByUser,
			Imported:             req.Imported,
			CidsToUpload:         map[cid.Cid]struct{}{},
			CidsToBind:           map[cid.Cid]struct{}{},
			MissingBlocksRetries: missingBlocksRetries,
		}
		return info, true, nil
	})
}

func (s *fileSync) MarkUploaded(objectId string) error {
	return s.process(objectId, func(exists bool, info FileInfo) (FileInfo, bool, error) {
		if !exists || (!info.State.IsUploadingState() && info.State != FileStateMissingBlocks) {
			return info, false, nil
		}
		info.State = FileStateDone
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
				logProcessingError("process next pending upload item", err)
			}
		}
	}
}

// runMissingBlocksRetrier re-processes items parked in MissingBlocks state:
// their blocks may have appeared since — uploaded to the node by another
// device, or cached locally by viewing the file
func (s *fileSync) runMissingBlocksRetrier(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := s.processNextPendingUploadItem(ctx, FileStateMissingBlocks)
			if err != nil && !errors.Is(err, filequeue.ErrClosed) {
				logProcessingError("process next missing-blocks item", err)
			}
		}
	}
}

// logProcessingError logs an expected degraded-environment condition (device
// offline, node unreachable) at WARN and everything else at ERROR
func logProcessingError(msg string, err error) {
	if isConnectivityError(err) {
		log.Warn(msg+": no connection to file node", zap.Error(err))
		return
	}
	log.Error(msg, zap.Error(err))
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
	if err != nil {
		err = fmt.Errorf("object %s, file %s: %w", item.ObjectId, item.FileId, err)
	}

	releaseErr := s.queue.ReleaseAndUpdate(item.ObjectId, next)

	return errors.Join(releaseErr, err)
}

func (s *fileSync) processFilePendingUpload(ctx context.Context, it FileInfo) (FileInfo, error) {
	blocksAvailability, err := s.checkBlocksAvailability(ctx, it)
	if err != nil {
		if errors.Is(err, errBlockNotFound) {
			return s.parkMissingBlocks(it, err), nil
		}
		it = rescheduleTransient(it, err)
		return it, fmt.Errorf("check blocks availability: %w", err)
	}

	it.BytesToUploadOrBind = blocksAvailability.bytesToUploadOrBind
	it.CidsToBind = blocksAvailability.cidsToBind
	it.CidsToUpload = blocksAvailability.cidsToUpload

	// The blocks were enumerated and checked successfully, so the
	// missing-blocks condition no longer holds: leave the parked state now, so
	// that a transient failure below gets the normal short retry instead of
	// the long missing-blocks backoff
	if it.State == FileStateMissingBlocks {
		it.State = FileStatePendingUpload
		it.MissingBlocksRetries = 0
	}

	spaceLimits, err := s.limitManager.getSpace(it.SpaceId)
	// If space is deleted, move file to deletion queue. It'll help to reclaim space if file is partially uploaded
	if errors.Is(err, errSpaceDeleted) {
		it.State = FileStatePendingDeletion

		return it, nil
	}
	if err != nil {
		it = rescheduleTransient(it, err)
		return it, fmt.Errorf("get space limits: %w", err)
	}

	allocateErr := spaceLimits.allocateFile(ctx, it.Key(), blocksAvailability.bytesToUploadOrBind)
	if allocateErr != nil {
		var limitErr *errLimitReached
		if !errors.As(allocateErr, &limitErr) {
			// Transient infra failure (e.g. SpaceInfo failed). Reschedule for
			// retry instead of marking the file as Limited.
			it = rescheduleTransient(it, allocateErr)
			return it, fmt.Errorf("allocate file: %w", allocateErr)
		}

		it.State = FileStateLimited
		it = it.Reschedule()

		err = s.handleLimitReached(ctx, it)
		if err != nil {
			if isObjectDeletedError(err) {
				it.State = FileStatePendingDeletion
				it.ScheduledAt = time.Now()
				return it, nil
			}
			return it, fmt.Errorf("handle limit reached: %w", err)
		}
		return it, nil
	}

	it, err = s.upload(ctx, it, blocksAvailability)
	if err != nil {
		spaceLimits.deallocateFile(it.Key())
		if isObjectDeletedError(err) {
			it.State = FileStatePendingDeletion
			it.ScheduledAt = time.Now()
			return it, nil
		}
		if errors.Is(err, errBlockNotFound) {
			return s.parkMissingBlocks(it, err), nil
		}
		it = rescheduleTransient(it, err)
		return it, err
	}

	switch it.State {
	case FileStateLimited, FileStatePendingDeletion:
		spaceLimits.deallocateFile(it.Key())
	case FileStateDone:
		spaceLimits.markFileUploaded(it.Key())
	default:
	}

	return it, nil
}

func (s *fileSync) upload(ctx context.Context, it FileInfo, blocksAvailability *blocksAvailabilityResponse) (FileInfo, error) {
	// Bind the blocks the node already has: needs no block data, so it works
	// even when the file is not present locally at all
	err := s.bindCids(ctx, it, blocksAvailability.orderedCids(blocksAvailability.cidsToBind))
	if err != nil {
		if isNodeLimitReachedError(err) {
			it.State = FileStateLimited

			err = s.handleLimitReached(ctx, it)
			if err != nil {
				return it, fmt.Errorf("handle limit reached: %w", err)
			}
			return it, nil
		}
		if isBlockNotFoundError(err) {
			// The node lost a block between BlocksCheck and BlocksBind (GC
			// race): drop the cached plan so the next attempt re-enumerates
			// and re-checks availability
			it.CidsToBind = nil
			it.CidsToUpload = nil
			it.BytesToUploadOrBind = 0
		}
		return it, fmt.Errorf("bind cids: %w", err)
	}
	// All cids should be bind at this time
	it.CidsToBind = nil

	// Means that we only had to bind blocks
	if len(blocksAvailability.cidsToUpload) == 0 {
		err = s.updateStatus(it, filesyncstatus.Synced)
		if err != nil {
			return it, fmt.Errorf("add to status update queue: %w", err)
		}
		it.State = FileStateDone
		return it, nil
	}

	err = s.uploadBlocks(ctx, it, blocksAvailability.orderedCids(blocksAvailability.cidsToUpload))
	if err != nil {
		return it, fmt.Errorf("upload blocks: %w", err)
	}

	if it.ObjectId != "" {
		err = s.updateStatus(it, filesyncstatus.Syncing)
		if err != nil {
			return it, fmt.Errorf("update status: %w", err)
		}
	}

	it.State = FileStateUploading
	return it, nil
}

func (s *fileSync) bindCids(ctx context.Context, it FileInfo, cids []cid.Cid) error {
	for start := 0; start < len(cids); start += bindBatchSize {
		end := min(start+bindBatchSize, len(cids))
		err := s.rpcStore.BindCids(ctx, it.SpaceId, it.FileId, cids[start:end])
		if err != nil {
			return fmt.Errorf("bind batch %d..%d of %d: %w", start, end, len(cids), err)
		}
	}
	return nil
}

// uploadBlocks pushes blocks to the file node in batches. Block data is read
// from the local store only: these blocks are missing on the node, so there is
// nowhere else to fetch them from.
func (s *fileSync) uploadBlocks(ctx context.Context, it FileInfo, cids []cid.Cid) error {
	dagService := s.dagServiceForSpace(it.SpaceId)
	batch := make([]blocks.Block, 0, uploadBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		err := s.requestsBatcher.addFile(it.SpaceId, it.FileId.String(), it.ObjectId, batch)
		if err != nil {
			return fmt.Errorf("add file to requests batcher: %w", err)
		}
		batch = make([]blocks.Block, 0, uploadBatchSize)
		return nil
	}
	for _, c := range cids {
		node, err := dagService.Get(ctx, c)
		if err != nil {
			if isBlockNotFoundError(err) {
				return fmt.Errorf("get local block %s: %w", c, errors.Join(err, errBlockNotFound))
			}
			return fmt.Errorf("get local block %s: %w", c, err)
		}
		b, err := blocks.NewBlockWithCid(node.RawData(), c)
		if err != nil {
			return fmt.Errorf("new block: %w", err)
		}
		batch = append(batch, b)
		if len(batch) == uploadBatchSize {
			if err = flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// parkMissingBlocks puts an item aside when its blocks are found neither
// locally nor on the file node: nothing can be done right now, but the blocks
// may appear later — another device uploads the file, or viewing the file
// caches its data locally — so the item is retried with a growing backoff
// instead of being dropped silently.
func (s *fileSync) parkMissingBlocks(it FileInfo, err error) FileInfo {
	log.Warn("file blocks are missing both locally and on the file node, parking for retry",
		zap.String("objectId", it.ObjectId),
		zap.String("fileId", it.FileId.String()),
		zap.String("spaceId", it.SpaceId),
		zap.Int("retries", it.MissingBlocksRetries),
		zap.Error(err))
	it.State = FileStateMissingBlocks
	it.ScheduledAt = time.Now().Add(missingBlocksRetryDelay(it.MissingBlocksRetries))
	it.MissingBlocksRetries++
	// Drop the cached availability plan: the situation may change while the
	// item is parked (another device uploads the file, node GC), and a retry
	// must re-enumerate and re-check instead of replaying a stale plan
	it.CidsToBind = nil
	it.CidsToUpload = nil
	it.BytesToUploadOrBind = 0
	return it
}

// rescheduleTransient delays an item after a transient failure. Items parked
// in MissingBlocks keep their long backoff instead of the 1-minute retry, and
// connectivity failures (device offline, node unreachable) get a longer delay
// than other transient errors — so an offline device doesn't churn its whole
// queue with dial attempts every minute.
func rescheduleTransient(it FileInfo, err error) FileInfo {
	if it.State == FileStateMissingBlocks {
		it.ScheduledAt = time.Now().Add(missingBlocksRetryDelay(it.MissingBlocksRetries))
		return it
	}
	if isConnectivityError(err) {
		it.ScheduledAt = time.Now().Add(connectivityRetryDelay)
		return it
	}
	return it.Reschedule()
}

func isConnectivityError(err error) bool {
	return errors.Is(err, rpcstore2.ErrNoConnectionToAnyFileClient) || errors.Is(err, net.ErrUnableToConnect)
}

func missingBlocksRetryDelay(retries int) time.Duration {
	const (
		baseDelay = 10 * time.Minute
		maxDelay  = 24 * time.Hour
	)
	delay := baseDelay
	for i := 0; i < retries && delay < maxDelay; i++ {
		delay *= 4
	}
	return min(delay, maxDelay)
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

const (
	// checkAvailabilityBatchSize limits the number of cids per BlocksCheck request
	checkAvailabilityBatchSize = 64
	// bindBatchSize limits the number of cids per BlocksBind request
	bindBatchSize = 64
	// uploadBatchSize limits how many blocks are handed to the requests
	// batcher at once
	uploadBatchSize = 10
)

// connectivityRetryDelay is used instead of the 1-minute reschedule when the
// file node cannot be reached at all
const connectivityRetryDelay = 5 * time.Minute

type blocksAvailabilityResponse struct {
	bytesToUploadOrBind int
	cidsToBind          map[cid.Cid]struct{}
	cidsToUpload        map[cid.Cid]struct{}
	// order holds the enumeration order of the whole DAG (small variants
	// first); empty for a plan restored from a persisted queue item
	order []cid.Cid
}

// orderedCids returns the members of the set in enumeration order when it is
// known, or in arbitrary order for a restored plan
func (r *blocksAvailabilityResponse) orderedCids(set map[cid.Cid]struct{}) []cid.Cid {
	out := make([]cid.Cid, 0, len(set))
	if len(r.order) > 0 {
		for _, c := range r.order {
			if _, ok := set[c]; ok {
				out = append(out, c)
			}
		}
		return out
	}
	for c := range set {
		out = append(out, c)
	}
	return out
}

// checkBlocksAvailability enumerates the file's cids (without requiring its
// data to be present locally, see enumerateFileCids) and asks the file node
// which of them it already has: those are bound to the space, the rest are
// uploaded from the local store.
func (s *fileSync) checkBlocksAvailability(ctx context.Context, info FileInfo) (*blocksAvailabilityResponse, error) {
	if len(info.CidsToBind) > 0 || len(info.CidsToUpload) > 0 {
		return &blocksAvailabilityResponse{
			bytesToUploadOrBind: info.BytesToUploadOrBind,
			cidsToBind:          info.CidsToBind,
			cidsToUpload:        info.CidsToUpload,
		}, nil
	}

	entries, err := s.enumerateFileCids(ctx, info.SpaceId, info.FileId, info.Variants)
	if err != nil {
		return nil, fmt.Errorf("enumerate file cids: %w", err)
	}

	response := blocksAvailabilityResponse{
		cidsToBind:   map[cid.Cid]struct{}{},
		cidsToUpload: map[cid.Cid]struct{}{},
		order:        make([]cid.Cid, 0, len(entries)),
	}
	sizes := make(map[cid.Cid]int, len(entries))
	for _, entry := range entries {
		response.order = append(response.order, entry.cid)
		sizes[entry.cid] = entry.size
	}

	for start := 0; start < len(entries); start += checkAvailabilityBatchSize {
		end := min(start+checkAvailabilityBatchSize, len(entries))
		batch := response.order[start:end]

		availabilities, err := s.rpcStore.CheckAvailability(ctx, info.SpaceId, batch)
		if err != nil {
			return nil, fmt.Errorf("check availability: %w", err)
		}
		statusByCid := make(map[cid.Cid]fileproto.AvailabilityStatus, len(availabilities))
		for _, availability := range availabilities {
			blockCid, err := cid.Cast(availability.Cid)
			if err != nil {
				return nil, fmt.Errorf("cast cid: %w", err)
			}
			statusByCid[blockCid] = availability.Status
		}
		for _, blockCid := range batch {
			status, answered := statusByCid[blockCid]
			// A cid the node did not answer for is treated as missing on the node
			if !answered {
				status = fileproto.AvailabilityStatus_NotExists
			}
			switch status {
			case fileproto.AvailabilityStatus_NotExists:
				response.cidsToUpload[blockCid] = struct{}{}
				response.bytesToUploadOrBind += sizes[blockCid]
			case fileproto.AvailabilityStatus_Exists:
				// Block exists on the node, but not in user's space
				response.cidsToBind[blockCid] = struct{}{}
				response.bytesToUploadOrBind += sizes[blockCid]
			}
		}
	}
	return &response, nil
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
	go func() {
		err := s.rpcStore.DeleteFiles(ctx, it.SpaceId, it.FileId)
		if err != nil {
			log.Error("calculate limits: unbind off-limit file", zap.String("fileId", it.FileId.String()), zap.Error(err))
		}
	}()

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
