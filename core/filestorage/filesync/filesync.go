package filesync

import (
	"context"
	"errors"
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
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/clientds"
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
	UpdateNodeUsage(ctx context.Context) error
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

func (s *fileSync) Init(a *app.App) (err error) {
	s.dbProvider = app.MustComponent[datastore.Datastore](a)
	s.rpcStore = a.MustComponent(rpcstore.CName).(rpcstore.Service).NewStore()
	s.dagService = a.MustComponent(fileservice.CName).(fileservice.FileService).DAGService()
	s.fileStore = app.MustComponent[filestore.FileStore](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.removePingCh = make(chan struct{})
	s.uploadPingCh = make(chan struct{})
	db, err := s.dbProvider.LocalStorage()
	if err != nil {
		return
	}
	s.uploadingQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, uploadingKeyPrefix, makeQueueItem), log.Logger, s.uploadingHandler)
	s.retryUploadingQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, retryUploadingKeyPrefix, makeQueueItem), log.Logger, s.retryingHandler, persistentqueue.WithHandlerTickPeriod(loopTimeout))
	s.deletionQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, deletionKeyPrefix, makeQueueItem), log.Logger, s.deletionHandler)
	s.retryDeletionQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, retryDeletionKeyPrefix, makeQueueItem), log.Logger, s.retryDeletionHandler, persistentqueue.WithHandlerTickPeriod(loopTimeout))
	return
}

func (s *fileSync) dagServiceForSpace(spaceID string) ipld.DAGService {
	return filehelper.NewDAGServiceWithSpaceID(spaceID, s.dagService)
}

func (s *fileSync) OnUploaded(callback StatusCallback) {
	s.onUploaded = callback
}

func (s *fileSync) OnUploadStarted(callback StatusCallback) {
	s.onUploadStarted = callback
}

func (s *fileSync) OnLimited(callback StatusCallback) {
	s.onLimited = callback
}

func (s *fileSync) Name() (name string) {
	return CName
}

func makeQueueItem() *QueueItem {
	return &QueueItem{}
}

func (s *fileSync) Run(ctx context.Context) (err error) {
	db, err := s.dbProvider.LocalStorage()
	if err != nil {
		if errors.Is(err, clientds.ErrSpaceStoreNotAvailable) {
			db, err = f.dbProvider.LocalStorage()
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	s.store, err = newFileSyncStore(db)
	if err != nil {
		return
	}

	s.uploadingQueue.Run()
	s.retryUploadingQueue.Run()
	s.deletionQueue.Run()
	s.retryDeletionQueue.Run()

	s.loopCtx, s.loopCancel = context.WithCancel(context.Background())
	go s.runNodeUsageUpdater()
	return
}

func (s *fileSync) Close(ctx context.Context) error {
	if s.loopCancel != nil {
		s.loopCancel()
	}
	// Don't wait
	go func() {
		if closer, ok := s.rpcStore.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				log.Error("can't close rpc store", zap.Error(err))
			}
		}
	}()

	if s.uploadingQueue != nil {
		if err := s.uploadingQueue.Close(); err != nil {
			log.Error("can't close uploading queue: %v", zap.Error(err))
		}
	}
	if s.retryUploadingQueue != nil {
		if err := s.retryUploadingQueue.Close(); err != nil {
			log.Error("can't close retrying queue: %v", zap.Error(err))
		}
	}
	if s.deletionQueue != nil {
		if err := s.deletionQueue.Close(); err != nil {
			log.Error("can't close deletion queue: %v", zap.Error(err))
		}
	}
	if s.retryDeletionQueue != nil {
		if err := s.retryDeletionQueue.Close(); err != nil {
			log.Error("can't close retry deletion queue: %v", zap.Error(err))
		}
	}

	return nil
}
