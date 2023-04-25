package filesync

import (
	"context"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonfile/fileservice"
	"github.com/cheggaaa/mb/v3"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/filestorage/rpcstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
)

const CName = "filesync"

var log = logger.NewNamed(CName)

var loopTimeout = time.Minute

func New() FileSync {
	return new(fileSync)
}

type FileSync interface {
	AddFile(spaceId, fileId string) (err error)
	RemoveFile(spaceId, fileId string) (err error)
	SpaceStat(ctx context.Context, spaceId string) (ss SpaceStat, err error)
	FileStat(ctx context.Context, spaceId, fileId string) (fs FileStat, err error)
	FileListStats(ctx context.Context, spaceId string, fileIDs []string) ([]FileStat, error)
	SyncStatus() (ss SyncStatus, err error)
	NewStatusWatcher(statusService StatusService, updateInterval time.Duration) *StatusWatcher
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

func (f *fileSync) AddFile(spaceId, fileId string) (err error) {
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
		spaceId, fileId, err := f.queue.GetUpload()
		if err != nil {
			if err != errQueueIsEmpty {
				log.Warn("queue get upload task error", zap.Error(err))
			}
			break
		}
		if err = f.uploadFile(f.loopCtx, spaceId, fileId); err != nil {
			log.Warn("upload file error", zap.Error(err), zap.String("fileID", fileId))
			break
		} else {
			if err = f.queue.DoneUpload(spaceId, fileId); err != nil {
				log.Warn("can't mark upload task as done", zap.Error(err))
				break
			}
		}
	}
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
	fileCid, err := cid.Parse(fileId)
	if err != nil {
		return
	}
	node, err := f.dagService.Get(ctx, fileCid)
	if err != nil {
		return
	}

	var (
		batcher = mb.New[blocks.Block](10)
		dagErr  = make(chan error, 1)
		bs      []blocks.Block
	)
	defer func() {
		_ = batcher.Close()
	}()

	go func() {
		defer func() {
			_ = batcher.Close()
		}()
		dagErr <- f.dagWalk(ctx, node, batcher)
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

func (f *fileSync) dagWalk(ctx context.Context, node ipld.Node, batcher *mb.MB[blocks.Block]) (err error) {
	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(node, f.dagService))
	err = walker.Iterate(func(node ipld.NavigableNode) error {
		b, err := blocks.NewBlockWithCid(node.GetIPLDNode().RawData(), node.GetIPLDNode().Cid())
		if err != nil {
			return err
		}
		if err = batcher.Add(ctx, b); err != nil {
			return err
		}
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

func (f *fileSync) Close(ctx context.Context) (err error) {
	if f.loopCancel != nil {
		f.loopCancel()
	}
	return
}
