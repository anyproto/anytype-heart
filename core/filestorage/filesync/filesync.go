package filesync

import (
	"context"
	"fmt"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonfile/fileproto"
	"github.com/anytypeio/any-sync/commonfile/fileservice"
	"github.com/cheggaaa/mb/v3"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/filestorage/rpcstore"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
)

const CName = "filesync"

var log = logger.NewNamed(CName)

var loopTimeout = time.Minute

var errReachedLimit = fmt.Errorf("file upload limit has been reached")

func New(sendEvent func(event *pb.Event)) FileSync {
	return &fileSync{
		sendEvent: sendEvent,
	}
}

//go:generate mockgen -package mock_filesync -destination ./mock_filesync/filesync_mock.go github.com/anytypeio/go-anytype-middleware/core/filestorage/filesync FileSync
type FileSync interface {
	AddFile(spaceId, fileId string) (err error)
	RemoveFile(spaceId, fileId string) (err error)
	SpaceStat(ctx context.Context, spaceId string) (ss SpaceStat, err error)
	FileStat(ctx context.Context, spaceId, fileId string) (fs FileStat, err error)
	FileListStats(ctx context.Context, spaceId string, fileIDs []string) ([]FileStat, error)
	SyncStatus() (ss SyncStatus, err error)
	FetchChunksCount(ctx context.Context, node ipld.Node) (int, error)
	HasUpload(spaceId, fileId string) (ok bool, err error)

	app.ComponentRunnable
}

type SyncStatus struct {
	QueueLen int
}

type fileSync struct {
	dbProvider   datastore.Datastore
	rpcStore     rpcstore.RpcStore
	queue        *fileSyncStore
	loopCtx      context.Context
	loopCancel   context.CancelFunc
	uploadPingCh chan struct{}
	removePingCh chan struct{}
	dagService   ipld.DAGService
	fileStore    filestore.FileStore
	sendEvent    func(event *pb.Event)
}

func (f *fileSync) Init(a *app.App) (err error) {
	f.dbProvider = a.MustComponent(datastore.CName).(datastore.Datastore)
	f.rpcStore = a.MustComponent(rpcstore.CName).(rpcstore.Service).NewStore()
	f.dagService = a.MustComponent(fileservice.CName).(fileservice.FileService).DAGService()
	f.fileStore = app.MustComponent[filestore.FileStore](a)
	f.removePingCh = make(chan struct{})
	f.uploadPingCh = make(chan struct{})
	return
}

func (f *fileSync) Name() (name string) {
	return CName
}

func (f *fileSync) Run(ctx context.Context) (err error) {
	db, err := f.dbProvider.SpaceStorage()
	if err != nil {
		return
	}
	f.queue = &fileSyncStore{db: db}
	f.loopCtx, f.loopCancel = context.WithCancel(context.Background())
	go f.addLoop()
	go f.removeLoop()
	return
}

func (f *fileSync) Close(ctx context.Context) (err error) {
	if f.loopCancel != nil {
		f.loopCancel()
	}
	return
}

func (f *fileSync) AddFile(spaceId, fileId string) (err error) {
	log.Info("add file to queue", zap.String("fileID", fileId))
	defer func() {
		if err == nil {
			select {
			case f.uploadPingCh <- struct{}{}:
			default:
			}
		}
	}()
	err = f.queue.QueueUpload(spaceId, fileId)
	return
}

func (f *fileSync) RemoveFile(spaceId, fileId string) (err error) {
	defer func() {
		if err == nil {
			select {
			case f.removePingCh <- struct{}{}:
			default:
			}
		}
	}()
	err = f.queue.QueueRemove(spaceId, fileId)
	return
}

func (f *fileSync) SyncStatus() (ss SyncStatus, err error) {
	ql, err := f.queue.QueueLen()
	if err != nil {
		return
	}
	return SyncStatus{
		QueueLen: ql,
	}, nil
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

func (f *fileSync) getUpload() (spaceId string, fileId string, wasDiscarded bool, err error) {
	spaceId, fileId, err = f.queue.GetUpload()
	if err == errQueueIsEmpty {
		spaceId, fileId, err = f.queue.GetDiscardedUpload()
		return spaceId, fileId, true, err
	}
	return spaceId, fileId, false, err
}

func (f *fileSync) tryToUpload() (string, error) {
	spaceId, fileId, wasDiscarded, err := f.getUpload()
	if err != nil {
		return fileId, err
	}
	if err = f.uploadFile(f.loopCtx, spaceId, fileId); err != nil {
		if err == errReachedLimit {
			if !wasDiscarded {
				f.sendLimitReachedEvent(spaceId, fileId)
			}
			log.Info("reached limit, push to discarded queue", zap.String("fileId", fileId))
			if qerr := f.queue.QueueDiscarded(spaceId, fileId); qerr != nil {
				log.Warn("can't push upload task to discarded queue", zap.String("fileId", fileId), zap.Error(qerr))
			}
			return fileId, err
		}

		ok, storeErr := f.hasFileInStore(fileId)
		if storeErr != nil {
			return fileId, storeErr
		}
		if !ok {
			log.Warn("file has been deleted from store, skip upload", zap.String("fileId", fileId))
			return fileId, f.queue.DoneUpload(spaceId, fileId)
		}
		// Push to the back of the queue
		if qerr := f.queue.QueueUpload(spaceId, fileId); qerr != nil {
			log.Warn("can't push upload task back to queue", zap.String("fileId", fileId), zap.Error(qerr))
		}
		return fileId, err
	}
	log.Info("done upload", zap.String("fileID", fileId))
	return fileId, f.queue.DoneUpload(spaceId, fileId)
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

func (f *fileSync) hasFileInStore(fileID string) (bool, error) {
	keys, err := f.fileStore.GetFileKeys(fileID)
	if err != localstore.ErrNotFound && err != nil {
		return false, err
	}
	return len(keys) > 0, nil
}

func (f *fileSync) removeLoop() {
	for {
		select {
		case <-f.loopCtx.Done():
			return
		case <-f.removePingCh:
		case <-time.After(loopTimeout):
		}
		for {
			spaceId, fileId, err := f.queue.GetRemove()
			if err != nil {
				if err != errQueueIsEmpty {
					log.Warn("queue get remove task error", zap.Error(err))
				}
				break
			}
			if err = f.removeFile(f.loopCtx, spaceId, fileId); err != nil {
				log.Warn("remove file error", zap.Error(err))
				break
			} else {
				if err = f.queue.DoneRemove(spaceId, fileId); err != nil {
					log.Warn("can't mark remove task as done", zap.Error(err))
					break
				}
			}
		}
	}
}

func (f *fileSync) uploadFile(ctx context.Context, spaceId, fileId string) (err error) {
	log.Info("uploading file", zap.String("fileId", fileId))

	var (
		batcher = mb.New[blocks.Block](10)
		dagErr  = make(chan error, 1)
		bs      []blocks.Block
	)
	defer func() {
		_ = batcher.Close()
	}()

	fileBlocks, err := f.collectFileBlocks(ctx, fileId)
	if err != nil {
		return fmt.Errorf("collect file blocks: %w", err)
	}

	bytesToUpload, blocksToUpload, err := f.selectBlocksToUploadAndBindExisting(ctx, spaceId, fileId, fileBlocks)
	if err != nil {
		return fmt.Errorf("select blocks to upload: %w", err)
	}

	stat, err := f.SpaceStat(ctx, spaceId)
	if err != nil {
		return fmt.Errorf("get space stat: %w", err)
	}

	bytesLeft := stat.BytesLimit - stat.BytesUsage
	if bytesToUpload > bytesLeft {
		return errReachedLimit
	}

	go func() {
		defer func() {
			_ = batcher.Close()
		}()
		proc := func() error {
			for _, b := range blocksToUpload {
				if addErr := batcher.Add(ctx, b); addErr != nil {
					return addErr
				}
			}
			return nil
		}
		dagErr <- proc()
	}()

	for {
		if bs, err = batcher.Wait(ctx); err != nil {
			if err == mb.ErrClosed {
				err = nil
				break
			} else {
				return err
			}
		}

		if err = f.rpcStore.AddToFile(ctx, spaceId, fileId, bs); err != nil {
			return err
		}
	}
	return <-dagErr
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

	if bindErr := f.rpcStore.BindCids(ctx, spaceId, fileId, cidsToBind); bindErr != nil {
		return 0, nil, fmt.Errorf("bind cids: %w", bindErr)
	}

	return bytesToUpload, blocksToUpload, nil
}

func (f *fileSync) collectFileBlocks(ctx context.Context, fileId string) (result []blocks.Block, err error) {
	fileCid, err := cid.Parse(fileId)
	if err != nil {
		return
	}
	node, err := f.dagService.Get(ctx, fileCid)
	if err != nil {
		return
	}

	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(node, f.dagService))
	err = walker.Iterate(func(node ipld.NavigableNode) error {
		b, err := blocks.NewBlockWithCid(node.GetIPLDNode().RawData(), node.GetIPLDNode().Cid())
		if err != nil {
			return err
		}
		result = append(result, b)
		return nil
	})
	if err == ipld.EndOfDag {
		err = nil
	}
	return
}

func (f *fileSync) removeFile(ctx context.Context, spaceId, fileId string) (err error) {
	return f.rpcStore.DeleteFiles(ctx, spaceId, fileId)
}

func (f *fileSync) HasUpload(spaceId, fileId string) (ok bool, err error) {
	return f.queue.HasUpload(spaceId, fileId)
}
