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
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
)

const CName = "filesync"

var log = logger.NewNamed(CName)

var loopTimeout = time.Minute

var errReachedLimit = fmt.Errorf("file upload limit has been reached")

type FileSync interface {
	AddFile(fileId domain.FullFileId, uploadedByUser, imported bool) (err error)
	UploadSynchronously(spaceId string, fileId domain.FileId) error
	OnUploadStarted(func(fileId domain.FileId) error)
	OnUploaded(func(fileId domain.FileId) error)
	OnLimited(func(fileId domain.FileId) error)
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
	onUploaded      func(fileId domain.FileId) error
	onUploadStarted func(fileId domain.FileId) error
	onLimited       func(fileId domain.FileId) error

	uploadingQueue *queue
	retryingQueue  *queue
	removingQueue  *queue

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

func (f *fileSync) OnUploaded(callback func(fileId domain.FileId) error) {
	f.onUploaded = callback
}

func (f *fileSync) OnUploadStarted(callback func(fileId domain.FileId) error) {
	f.onUploadStarted = callback
}

func (f *fileSync) OnLimited(callback func(fileId domain.FileId) error) {
	f.onLimited = callback
}

func (f *fileSync) Name() (name string) {
	return CName
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

	{
		q, err := newQueue(db, uploadKeyPrefix, uploadKey)
		if err != nil {
			return fmt.Errorf("new uploading queue: %w", err)
		}
		f.uploadingQueue = q
	}

	{
		q, err := newQueue(db, discardedKeyPrefix, discardedKey)
		if err != nil {
			return fmt.Errorf("new retrying queue: %w", err)
		}
		f.retryingQueue = q
	}

	{
		q, err := newQueue(db, removeKeyPrefix, removeKey)
		if err != nil {
			return fmt.Errorf("new removing queue: %w", err)
		}
		f.removingQueue = q
	}

	f.loopCtx, f.loopCancel = context.WithCancel(context.Background())
	go f.runNodeUsageUpdater()
	go f.uploadLoop()
	go f.uploadDiscardedLoop()
	go f.removeLoop()
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

	if err := f.uploadingQueue.close(); err != nil {
		log.Error("can't close uploading queue: %v", zap.Error(err))
	}
	if err := f.retryingQueue.close(); err != nil {
		log.Error("can't close retrying queue: %v", zap.Error(err))
	}
	if err := f.removingQueue.close(); err != nil {
		log.Error("can't close removing queue: %v", zap.Error(err))
	}

	return nil
}

func (f *fileSync) SyncStatus() (ss SyncStatus, err error) {
	return SyncStatus{
		QueueLen: f.uploadingQueue.length(),
	}, nil
}
