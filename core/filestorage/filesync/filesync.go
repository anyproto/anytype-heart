package filesync

import (
	"context"
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
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

const CName = "filesync"

var log = logger.NewNamed(CName)

var loopTimeout = time.Minute

type StatusCallback func(fileObjectId string) error

type FileSync interface {
	AddFile(fileObjectId string, fileId domain.FullFileId, uploadedByUser, imported bool) (err error)
	UploadSynchronously(spaceId string, fileId domain.FileId) error
	OnUploadStarted(StatusCallback)
	OnUploaded(StatusCallback)
	OnLimited(StatusCallback)
	DeleteFile(objectId string, fileId domain.FullFileId) (err error)
	DeleteFileSynchronously(fileId domain.FullFileId) (err error)
	NodeUsage(ctx context.Context) (usage NodeUsage, err error)
	SpaceStat(ctx context.Context, spaceId string) (ss SpaceStat, err error)
	FileStat(ctx context.Context, spaceId string, fileId domain.FileId) (fs FileStat, err error)
	FileListStats(ctx context.Context, spaceId string, hashes []domain.FileId) ([]FileStat, error)
	DebugQueue(*http.Request) (*QueueInfo, error)
	SendImportEvents()
	ClearImportEvents()
	CalculateFileSize(ctx context.Context, spaceId string, fileId domain.FileId) (int, error)
	app.ComponentRunnable
}

type QueueInfo struct {
	UploadingQueue      []string
	RetryUploadingQueue []string
	DeletionQueue       []string
	RetryDeletionQueue  []string
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

	uploadingQueue      *persistentqueue.Queue[*QueueItem]
	retryUploadingQueue *persistentqueue.Queue[*QueueItem]
	deletionQueue       *persistentqueue.Queue[*QueueItem]
	retryDeletionQueue  *persistentqueue.Queue[*QueueItem]

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
	db, err := f.dbProvider.LocalStorage()
	if err != nil {
		return
	}
	f.uploadingQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, uploadingKeyPrefix, makeQueueItem), log.Logger, f.uploadingHandler)
	f.retryUploadingQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, retryUploadingKeyPrefix, makeQueueItem), log.Logger, f.retryingHandler, persistentqueue.WithHandlerTickPeriod(loopTimeout))
	f.deletionQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, deletionKeyPrefix, makeQueueItem), log.Logger, f.deletionHandler)
	f.retryDeletionQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, retryDeletionKeyPrefix, makeQueueItem), log.Logger, f.retryDeletionHandler, persistentqueue.WithHandlerTickPeriod(loopTimeout))
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
	db, err := f.dbProvider.LocalStorage()
	if err != nil {
		return
	}
	f.store, err = newFileSyncStore(db)
	if err != nil {
		return
	}

	f.uploadingQueue.Run()
	f.retryUploadingQueue.Run()
	f.deletionQueue.Run()
	f.retryDeletionQueue.Run()

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
	if err := f.retryUploadingQueue.Close(); err != nil {
		log.Error("can't close retrying queue: %v", zap.Error(err))
	}
	if err := f.deletionQueue.Close(); err != nil {
		log.Error("can't close deletion queue: %v", zap.Error(err))
	}
	if err := f.retryDeletionQueue.Close(); err != nil {
		log.Error("can't close retry deletion queue: %v", zap.Error(err))
	}

	return nil
}
