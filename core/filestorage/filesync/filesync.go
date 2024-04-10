package filesync

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	ipld "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/queue"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
)

const CName = "filesync"

var log = logger.NewNamed(CName)

var loopTimeout = time.Minute

var errReachedLimit = fmt.Errorf("file upload limit has been reached")

type StatusCallback func(fileObjectId string) error

type FileSync interface {
	AddFile(fileObjectId string, fileId domain.FullFileId, uploadedByUser, imported bool) (err error)
	UploadSynchronously(spaceId string, fileId domain.FileId) error
	OnUploadStarted(StatusCallback)
	OnUploaded(StatusCallback)
	OnLimited(StatusCallback)
	RemoveFile(fileId domain.FullFileId) (err error)
	RemoveSynchronously(spaceId string, fileId domain.FileId) (err error)
	NodeUsage(ctx context.Context) (usage NodeUsage, err error)
	SpaceStat(ctx context.Context, spaceId string) (ss SpaceStat, err error)
	FileStat(ctx context.Context, spaceId string, fileId domain.FileId) (fs FileStat, err error)
	FileListStats(ctx context.Context, spaceId string, hashes []domain.FileId) ([]FileStat, error)
	SyncStatus() (ss SyncStatus, err error)
	DebugQueue(*http.Request) (*QueueInfo, error)
	SendImportEvents()
	ClearImportEvents()
	CalculateFileSize(ctx context.Context, spaceId string, fileId domain.FileId) (int, error)
	app.ComponentRunnable
}

type QueueInfo struct {
	UploadingQueue []*QueueItem
	DiscardedQueue []*QueueItem
	RemovingQueue  []*QueueItem
}

type SyncStatus struct {
	QueueLen int
}

type fileSync struct {
	store           *fileSyncStore
	dbProvider      datastore.Datastore
	rpcStore        rpcstore.RpcStore
	loopCtx         context.Context
	loopCancel      context.CancelFunc
	uploadPingCh    chan struct{}
	removePingCh    chan struct{}
	dagService      ipld.DAGService
	fileStore       filestore.FileStore
	eventSender     event.Sender
	onUploaded      StatusCallback
	onUploadStarted StatusCallback
	onLimited       StatusCallback

	uploadingQueue *queue.Queue[*QueueItem]
	retryingQueue  *queue.Queue[*QueueItem]
	removingQueue  *queue.Queue[*QueueItem]

	importEventsMutex sync.Mutex
	importEvents      []*pb.Event
}

func New() FileSync {
	return &fileSync{}
}

func (f *fileSync) Init(a *app.App) (err error) {
	f.dbProvider = app.MustComponent[datastore.Datastore](a)
	f.rpcStore = a.MustComponent(rpcstore.CName).(rpcstore.Service).NewStore()
	f.dagService = a.MustComponent(fileservice.CName).(fileservice.FileService).DAGService()
	f.fileStore = app.MustComponent[filestore.FileStore](a)
	f.eventSender = app.MustComponent[event.Sender](a)
	f.removePingCh = make(chan struct{})
	f.uploadPingCh = make(chan struct{})
	return
}

func (f *fileSync) dagServiceForSpace(spaceID string) ipld.DAGService {
	return filehelper.NewDAGServiceWithSpaceID(spaceID, f.dagService)
}

func (f *fileSync) OnUploaded(callback StatusCallback) {
	f.onUploaded = callback
}

func (f *fileSync) OnUploadStarted(callback StatusCallback) {
	f.onUploadStarted = callback
}

func (f *fileSync) OnLimited(callback StatusCallback) {
	f.onLimited = callback
}

func (f *fileSync) Name() (name string) {
	return CName
}

func makeQueueItem() *QueueItem {
	return &QueueItem{}
}

func (f *fileSync) Run(ctx context.Context) (err error) {
	db, err := f.dbProvider.SpaceStorage()
	if err != nil {
		return
	}
	f.store, err = newFileSyncStore(db)
	if err != nil {
		return
	}

	f.uploadingQueue = queue.New(db, log.Logger, uploadKeyPrefix, makeQueueItem, f.uploadingHandler)
	f.uploadingQueue.Run()
	f.retryingQueue = queue.New(db, log.Logger, discardedKeyPrefix, makeQueueItem, f.uploadingHandler, queue.WithHandlerTickPeriod(loopTimeout))
	f.retryingQueue.Run()
	f.removingQueue = queue.New(db, log.Logger, removeKeyPrefix, makeQueueItem, f.removingHandler)
	f.removingQueue.Run()

	f.loopCtx, f.loopCancel = context.WithCancel(context.Background())
	go f.runNodeUsageUpdater()
	return
}

func (f *fileSync) Close(ctx context.Context) error {
	if f.loopCancel != nil {
		f.loopCancel()
	}
	// Don't wait
	go func() {
		if closer, ok := f.rpcStore.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				log.Error("can't close rpc store", zap.Error(err))
			}
		}
	}()

	if err := f.uploadingQueue.Close(); err != nil {
		log.Error("can't close uploading queue: %v", zap.Error(err))
	}
	if err := f.retryingQueue.Close(); err != nil {
		log.Error("can't close retrying queue: %v", zap.Error(err))
	}
	if err := f.removingQueue.Close(); err != nil {
		log.Error("can't close removing queue: %v", zap.Error(err))
	}

	return nil
}

func (f *fileSync) SyncStatus() (ss SyncStatus, err error) {
	return SyncStatus{
		QueueLen: f.uploadingQueue.Len(),
	}, nil
}
