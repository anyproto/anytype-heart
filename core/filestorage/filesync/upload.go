package filesync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
)

func (f *fileSync) AddFile(spaceID string, fileId domain.FileId, uploadedByUser bool, imported bool) (err error) {
	err = f.queue.QueueUpload(spaceID, fileId, uploadedByUser, imported)
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
		f.eventSender.Broadcast(event)
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
		fileId, err := f.tryToUpload()
		if err == errQueueIsEmpty {
			return
		}
		if err != nil {
			log.Warn("can't upload file", zap.String("fileId", fileId.String()), zap.Error(err))
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

func (f *fileSync) tryToUpload() (domain.FileId, error) {
	it, err := f.getUpload()
	if err != nil {
		return "", err
	}
	spaceId, fileId := it.SpaceId, it.FileId
	f.runOnUploadStartedHook(fileId, spaceId)
	if err = f.uploadFile(f.loopCtx, spaceId, fileId); err != nil {
		if isLimitReachedErr(err) {
			f.runOnLimitedHook(fileId, spaceId)

			if it.AddedByUser && !it.Imported {
				f.sendLimitReachedEvent(spaceId, fileId)
			}
			if it.Imported {
				f.addImportEvent(spaceId, fileId)
			}
			if qerr := f.queue.QueueDiscarded(spaceId, fileId); qerr != nil {
				log.Warn("can't push upload task to discarded queue", zap.String("fileId", fileId.String()), zap.Error(qerr))
			}
			return fileId, err
		}

		// Push to the back of the queue
		if qerr := f.queue.QueueUpload(spaceId, fileId, it.AddedByUser, it.Imported); qerr != nil {
			log.Warn("can't push upload task back to queue", zap.String("fileId", fileId.String()), zap.Error(qerr))
		}
		return fileId, err
	}
	f.runOnUploadedHook(fileId, spaceId)

	f.updateSpaceUsageInformation(spaceId)

	return fileId, f.queue.DoneUpload(spaceId, fileId)
}

func (f *fileSync) UploadSynchronously(spaceId string, fileId domain.FileId) error {
	f.runOnUploadStartedHook(fileId, spaceId)
	err := f.uploadFile(context.Background(), spaceId, fileId)
	if err != nil {
		return err
	}
	f.runOnUploadedHook(fileId, spaceId)
	f.updateSpaceUsageInformation(spaceId)
	return nil
}

func (f *fileSync) runOnUploadedHook(fileId domain.FileId, spaceId string) {
	if f.onUploaded != nil {
		err := f.onUploaded(fileId)
		if err != nil {
			log.Warn("on upload callback failed",
				zap.String("fileId", fileId.String()),
				zap.String("spaceID", spaceId),
				zap.Error(err))
		}
	}
}

func (f *fileSync) runOnUploadStartedHook(fileId domain.FileId, spaceId string) {
	if f.onUploadStarted != nil {
		err := f.onUploadStarted(fileId)
		if err != nil {
			log.Warn("on upload started callback failed",
				zap.String("fileId", fileId.String()),
				zap.String("spaceID", spaceId),
				zap.Error(err))
		}
	}
}

func (f *fileSync) runOnLimitedHook(fileId domain.FileId, spaceId string) {
	if f.onLimited != nil {
		err := f.onLimited(fileId)
		if err != nil {
			log.Warn("on limited callback failed",
				zap.String("fileId", fileId.String()),
				zap.String("spaceID", spaceId),
				zap.Error(err))
		}
	}
}

func isLimitReachedErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, errReachedLimit) || strings.Contains(err.Error(), fileprotoerr.ErrSpaceLimitExceeded.Error())
}

func (f *fileSync) uploadFile(ctx context.Context, spaceID string, fileId domain.FileId) error {
	log.Debug("uploading file", zap.String("fileId", fileId.String()))

	fileSize, err := f.CalculateFileSize(ctx, spaceID, fileId)
	if err != nil {
		return fmt.Errorf("calculate file size: %w", err)
	}
	stat, err := f.getAndUpdateSpaceStat(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("get space stat: %w", err)
	}

	bytesLeft := stat.AccountBytesLimit - stat.TotalBytesUsage
	if fileSize > bytesLeft {
		return errReachedLimit
	}

	var totalBytesUploaded int
	err = f.walkFileBlocks(ctx, spaceID, fileId, func(fileBlocks []blocks.Block) error {
		bytesToUpload, blocksToUpload, err := f.selectBlocksToUploadAndBindExisting(ctx, spaceID, fileId, fileBlocks)
		if err != nil {
			return fmt.Errorf("select blocks to upload: %w", err)
		}
		if err = f.rpcStore.AddToFile(ctx, spaceID, fileId, blocksToUpload); err != nil {
			return err
		}
		totalBytesUploaded += bytesToUpload
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk file blocks: %w", err)
	}

	log.Warn("done upload", zap.String("fileId", fileId.String()), zap.Int("estimatedSize", fileSize), zap.Int("bytesUploaded", totalBytesUploaded))

	return nil
}

func (f *fileSync) sendLimitReachedEvent(spaceID string, fileId domain.FileId) {
	f.eventSender.Broadcast(&pb.Event{
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

func (f *fileSync) addImportEvent(spaceID string, fileId domain.FileId) {
	f.importEventsMutex.Lock()
	defer f.importEventsMutex.Unlock()
	f.importEvents = append(f.importEvents, &pb.Event{
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

func (f *fileSync) selectBlocksToUploadAndBindExisting(ctx context.Context, spaceId string, fileId domain.FileId, fileBlocks []blocks.Block) (int, []blocks.Block, error) {
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

func (f *fileSync) walkDAG(ctx context.Context, spaceId string, fileId domain.FileId, visit func(node ipld.Node) error) error {
	fileCid, err := cid.Parse(fileId.String())
	if err != nil {
		return fmt.Errorf("parse CID %s: %w", fileId, err)
	}
	dagService := f.dagServiceForSpace(spaceId)
	rootNode, err := dagService.Get(ctx, fileCid)
	if err != nil {
		return fmt.Errorf("get root node: %w", err)
	}

	visited := map[cid.Cid]struct{}{}
	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(rootNode, dagService))
	err = walker.Iterate(func(navNode ipld.NavigableNode) error {
		node := navNode.GetIPLDNode()
		if _, ok := visited[node.Cid()]; !ok {
			visited[node.Cid()] = struct{}{}
			return visit(node)
		}
		return nil
	})
	if errors.Is(err, ipld.EndOfDag) {
		err = nil
	}
	return err
}

// CalculateFileSize calculates or gets already calculated file size
func (f *fileSync) CalculateFileSize(ctx context.Context, spaceId string, fileId domain.FileId) (int, error) {
	size, err := f.fileStore.GetFileSize(fileId)
	if err == nil {
		return size, nil
	}

	size = 0
	err = f.walkDAG(ctx, spaceId, fileId, func(node ipld.Node) error {
		size += len(node.RawData())
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("walk DAG: %w", err)
	}
	err = f.fileStore.SetFileSize(fileId, size)
	if err != nil {
		log.Error("can't store file size", zap.String("fileId", fileId.String()), zap.Error(err))
	}
	return size, nil
}

const batchSize = 10

func (f *fileSync) walkFileBlocks(ctx context.Context, spaceId string, fileId domain.FileId, proc func(fileBlocks []blocks.Block) error) error {
	blocksBuf := make([]blocks.Block, 0, batchSize)

	err := f.walkDAG(ctx, spaceId, fileId, func(node ipld.Node) error {
		b, err := blocks.NewBlockWithCid(node.RawData(), node.Cid())
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

func (f *fileSync) HasUpload(spaceId string, fileId domain.FileId) (ok bool, err error) {
	return f.queue.HasUpload(spaceId, fileId)
}

func (f *fileSync) IsFileUploadLimited(spaceId string, fileId domain.FileId) (ok bool, err error) {
	return f.queue.IsFileUploadLimited(spaceId, fileId)
}
