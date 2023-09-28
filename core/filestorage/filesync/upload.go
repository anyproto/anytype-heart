package filesync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
)

func (f *fileSync) AddFile(spaceID, fileID string, uploadedByUser, imported bool) (err error) {
	status, err := f.fileStore.GetSyncStatus(fileID)
	if err != nil && !errors.Is(err, localstore.ErrNotFound) {
		return fmt.Errorf("get file sync status: %w", err)
	}
	if status == int(syncstatus.StatusSynced) {
		return nil
	}
	ok, storeErr := f.hasFileInStore(fileID)
	if storeErr != nil {
		return fmt.Errorf("check if file is in store: %w", storeErr)
	}
	if !ok {
		log.Warn("file has been deleted from store, skip upload", zap.String("fileID", fileID))
		return nil
	}

	err = f.queue.QueueUpload(spaceID, fileID, uploadedByUser, imported)
	if err == nil {
		select {
		case f.uploadPingCh <- struct{}{}:
		default:
		}
	}
	return
}

func (f *fileSync) SendImportEvents() {
	f.importEventsMutex.Lock()
	defer f.importEventsMutex.Unlock()
	for _, event := range f.importEvents {
		f.sendEvent(event)
	}
}

func (f *fileSync) ClearImportEvents() {
	f.importEventsMutex.Lock()
	defer f.importEventsMutex.Unlock()
	f.importEvents = nil
}

func (f *fileSync) addLoop() {
	f.addOperation()
	for {
		select {
		case <-f.loopCtx.Done():
			return
		case <-f.uploadPingCh:
		case <-time.After(loopTimeout):
		}
		f.addOperation()
	}
}

func (f *fileSync) addOperation() {
	for {
		fileID, err := f.tryToUpload()
		if err == errQueueIsEmpty {
			return
		}
		if err != nil {
			log.Warn("can't upload file", zap.String("fileID", fileID), zap.Error(err))
			return
		}
	}
}

func (f *fileSync) getUpload() (*QueueItem, error) {
	it, err := f.queue.GetUpload()
	if err == errQueueIsEmpty {
		return f.queue.GetDiscardedUpload()
	}
	return it, err
}

func (f *fileSync) tryToUpload() (string, error) {
	it, err := f.getUpload()
	if err != nil {
		return "", err
	}
	spaceId, fileId := it.SpaceID, it.FileID
	ok, storeErr := f.hasFileInStore(fileId)
	if storeErr != nil {
		return fileId, fmt.Errorf("check if file is in store: %w", storeErr)
	}
	if !ok {
		log.Warn("file has been deleted from store, skip upload", zap.String("fileId", fileId))
		return fileId, f.queue.DoneUpload(spaceId, fileId)
	}
	if err = f.uploadFile(f.loopCtx, spaceId, fileId); err != nil {
		if isLimitReachedErr(err) {
			if it.AddedByUser && !it.Imported {
				f.sendLimitReachedEvent(spaceId, fileId)
			}
			if it.Imported {
				f.addImportEvent(spaceId, fileId)
			}
			if qerr := f.queue.QueueDiscarded(spaceId, fileId); qerr != nil {
				log.Warn("can't push upload task to discarded queue", zap.String("fileId", fileId), zap.Error(qerr))
			}
			return fileId, err
		}

		// Push to the back of the queue
		if qerr := f.queue.QueueUpload(spaceId, fileId, it.AddedByUser, it.Imported); qerr != nil {
			log.Warn("can't push upload task back to queue", zap.String("fileId", fileId), zap.Error(qerr))
		}
		return fileId, err
	}
	if f.onUpload != nil {
		err := f.onUpload(spaceId, fileId)
		if err != nil {
			log.Warn("on upload callback failed",
				zap.String("fileID", fileId),
				zap.String("spaceID", spaceId),
				zap.Error(err))
		}
	}

	f.updateSpaceUsageInformation(spaceId)

	return fileId, f.queue.DoneUpload(spaceId, fileId)
}

func isLimitReachedErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errReachedLimit) || strings.Contains(err.Error(), fileprotoerr.ErrSpaceLimitExceeded.Error())
}

func (f *fileSync) uploadFile(ctx context.Context, spaceID string, fileID string) error {
	log.Debug("uploading file", zap.String("fileID", fileID))

	fileSize, err := f.calculateFileSize(ctx, fileID)
	if err != nil {
		return fmt.Errorf("calculate file size: %w", err)
	}
	stat, err := f.getAndUpdateSpaceStat(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("get space stat: %w", err)
	}

	bytesLeft := stat.BytesLimit - stat.BytesUsage
	if fileSize > bytesLeft {
		return errReachedLimit
	}

	var totalBytesUploaded int
	err = f.walkFileBlocks(ctx, fileID, func(fileBlocks []blocks.Block) error {
		bytesToUpload, blocksToUpload, err := f.selectBlocksToUploadAndBindExisting(ctx, spaceID, fileID, fileBlocks)
		if err != nil {
			return fmt.Errorf("select blocks to upload: %w", err)
		}
		if err = f.rpcStore.AddToFile(ctx, spaceID, fileID, blocksToUpload); err != nil {
			return err
		}
		totalBytesUploaded += bytesToUpload
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk file blocks: %w", err)
	}

	log.Warn("done upload", zap.String("fileID", fileID), zap.Int("estimatedSize", fileSize), zap.Int("bytesUploaded", totalBytesUploaded))

	return nil
}

func (f *fileSync) hasFileInStore(fileID string) (bool, error) {
	roots, err := f.fileStore.ListByTarget(fileID)
	if err != localstore.ErrNotFound && err != nil {
		return false, err
	}
	return len(roots) > 0, nil
}

func (f *fileSync) sendLimitReachedEvent(spaceID string, fileID string) {
	f.sendEvent(&pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfFileLimitReached{
					FileLimitReached: &pb.EventFileLimitReached{
						SpaceId: spaceID,
						FileId:  fileID,
					},
				},
			},
		},
	})
}

func (f *fileSync) addImportEvent(spaceID string, fileID string) {
	f.importEventsMutex.Lock()
	defer f.importEventsMutex.Unlock()
	f.importEvents = append(f.importEvents, &pb.Event{
		Messages: []*pb.EventMessage{
			{
				Value: &pb.EventMessageValueOfFileLimitReached{
					FileLimitReached: &pb.EventFileLimitReached{
						SpaceId: spaceID,
						FileId:  fileID,
					},
				},
			},
		},
	})
}

func (f *fileSync) selectBlocksToUploadAndBindExisting(ctx context.Context, spaceId string, fileId string, fileBlocks []blocks.Block) (int, []blocks.Block, error) {
	fileCids := lo.Map(fileBlocks, func(b blocks.Block, _ int) cid.Cid {
		return b.Cid()
	})
	availabilities, err := f.rpcStore.CheckAvailability(ctx, spaceId, fileCids)
	if err != nil {
		return 0, nil, fmt.Errorf("check availabilit: %w", err)
	}

	var (
		bytesToUpload  int
		blocksToUpload []blocks.Block
		cidsToBind     []cid.Cid
	)
	for _, availability := range availabilities {
		blockCid, err := cid.Cast(availability.Cid)
		if err != nil {
			return 0, nil, fmt.Errorf("cast cid: %w", err)
		}

		if availability.Status == fileproto.AvailabilityStatus_NotExists {
			b, ok := lo.Find(fileBlocks, func(b blocks.Block) bool {
				return b.Cid() == blockCid
			})
			if !ok {
				return 0, nil, fmt.Errorf("block %s not found", blockCid)
			}

			blocksToUpload = append(blocksToUpload, b)
			bytesToUpload += len(b.RawData())
		} else {
			cidsToBind = append(cidsToBind, blockCid)
		}
	}

	if len(cidsToBind) > 0 {
		if bindErr := f.rpcStore.BindCids(ctx, spaceId, fileId, cidsToBind); bindErr != nil {
			return 0, nil, fmt.Errorf("bind cids: %w", bindErr)
		}
	}
	return bytesToUpload, blocksToUpload, nil
}

func (f *fileSync) walkDAG(ctx context.Context, fileID string, visit func(node ipld.NavigableNode) error) error {
	fileCid, err := cid.Parse(fileID)
	if err != nil {
		return fmt.Errorf("parse CID %s: %w", fileID, err)
	}
	node, err := f.dagService.Get(ctx, fileCid)
	if err != nil {
		return fmt.Errorf("get root node: %w", err)
	}

	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(node, f.dagService))
	err = walker.Iterate(visit)
	if errors.Is(err, ipld.EndOfDag) {
		err = nil
	}
	return err
}

func (f *fileSync) calculateFileSize(ctx context.Context, fileID string) (int, error) {
	size, err := f.fileStore.GetFileSize(fileID)
	if err == nil {
		return size, nil
	}

	size = 0
	err = f.walkDAG(ctx, fileID, func(node ipld.NavigableNode) error {
		raw := node.GetIPLDNode().RawData()
		size += len(raw)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk DAG: %w", err)
	}
	err = f.fileStore.SetFileSize(fileID, size)
	if err != nil {
		log.Error("can't store file size", zap.String("fileID", fileID), zap.Error(err))
	}
	return size, nil
}

const batchSize = 10

func (f *fileSync) walkFileBlocks(ctx context.Context, fileID string, proc func(fileBlocks []blocks.Block) error) error {
	blocksBuf := make([]blocks.Block, 0, batchSize)

	err := f.walkDAG(ctx, fileID, func(node ipld.NavigableNode) error {
		b, err := blocks.NewBlockWithCid(node.GetIPLDNode().RawData(), node.GetIPLDNode().Cid())
		if err != nil {
			return err
		}
		blocksBuf = append(blocksBuf, b)
		if len(blocksBuf) == batchSize {
			err = proc(blocksBuf)
			if err != nil {
				return fmt.Errorf("process batch: %w", err)
			}
			blocksBuf = blocksBuf[:0]
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk DAG: %w", err)
	}

	if len(blocksBuf) > 0 {
		err = proc(blocksBuf)
		if err != nil {
			return fmt.Errorf("process batch: %w", err)
		}
	}
	return nil
}

func (f *fileSync) HasUpload(spaceId, fileId string) (ok bool, err error) {
	return f.queue.HasUpload(spaceId, fileId)
}

func (f *fileSync) IsFileUploadLimited(spaceId, fileId string) (ok bool, err error) {
	return f.queue.IsFileUploadLimited(spaceId, fileId)
}
