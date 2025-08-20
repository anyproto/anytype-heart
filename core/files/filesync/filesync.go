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
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	rpcstore2 "github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
	"github.com/anyproto/anytype-heart/util/timeid"
)

const CName = "filesync"

var log = logger.NewNamed(CName)

var loopTimeout = time.Minute

type StatusCallback func(fileObjectId string, fileId domain.FullFileId, status filesyncstatus.Status) error

type FileSync interface {
	AddFile(req AddFileRequest) (err error)
	UploadSynchronously(ctx context.Context, spaceId string, fileId domain.FileId) error
	OnStatusUpdated(StatusCallback)
	CancelDeletion(objectId string, fileId domain.FullFileId) (err error)
	DeleteFile(objectId string, fileId domain.FullFileId) (err error)
	DeleteFileSynchronously(fileId domain.FullFileId) (err error)
	UpdateNodeUsage(ctx context.Context) error
	NodeUsage(ctx context.Context) (usage NodeUsage, err error)
	SpaceStat(ctx context.Context, spaceId string) (ss SpaceStat, err error)
	DebugQueue(*http.Request) (*QueueInfo, error)
	SendImportEvents()
	ClearImportEvents()
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

type statusUpdateItem struct {
	FileObjectId string
	FileId       string
	SpaceId      string
	Timestamp    int64
	Status       int
}

func (it *statusUpdateItem) Key() string {
	return it.FileObjectId
}

type fileSync struct {
	rpcStore        rpcstore2.RpcStore
	loopCtx         context.Context
	loopCancel      context.CancelFunc
	dagService      ipld.DAGService
	eventSender     event.Sender
	onStatusUpdated []StatusCallback

	uploadingQueue        *persistentqueue.Queue[*QueueItem]
	retryUploadingQueue   *persistentqueue.Queue[*QueueItem]
	limitedUploadingQueue *persistentqueue.Queue[*QueueItem]

	deletionQueue      *persistentqueue.Queue[*deletionQueueItem]
	retryDeletionQueue *persistentqueue.Queue[*deletionQueueItem]
	statusUpdateQueue  *persistentqueue.Queue[*statusUpdateItem]

	blocksAvailabilityCache   keyvaluestore.Store[*blocksAvailabilityResponse]
	isLimitReachedErrorLogged keyvaluestore.Store[bool]
	nodeUsageCache            keyvaluestore.Store[NodeUsage]
	pendingUploads            keyvaluestore.Store[*QueueItem]

	limitManager      *uploadLimitManager
	uploadStatusIndex *uploadStatusIndex
	requestsBatcher   *requestsBatcher
	requestsCh        chan *fileproto.BlockPushManyRequest

	importEventsMutex sync.Mutex
	importEvents      []*pb.Event
	cfg               *config.Config

	closeWg *sync.WaitGroup
}

func New() FileSync {
	return &fileSync{closeWg: &sync.WaitGroup{}}
}

func (s *fileSync) Init(a *app.App) (err error) {
	s.loopCtx, s.loopCancel = context.WithCancel(context.Background())
	s.rpcStore = app.MustComponent[rpcstore2.Service](a).NewStore()
	s.dagService = app.MustComponent[fileservice.FileService](a).DAGService()
	s.eventSender = app.MustComponent[event.Sender](a)
	s.cfg = app.MustComponent[*config.Config](a)
	s.limitManager = newLimitManager(s.rpcStore)

	provider := app.MustComponent[anystoreprovider.Provider](a)
	db := provider.GetCommonDb()

	s.blocksAvailabilityCache, err = keyvaluestore.NewJson[*blocksAvailabilityResponse](provider.GetCommonDb(), "filesync/bytes_to_upload")
	if err != nil {
		return fmt.Errorf("init blocks availability cache: %w", err)
	}
	s.isLimitReachedErrorLogged, err = keyvaluestore.NewJson[bool](db, "filesync/limit_reached_error_logged")
	if err != nil {
		return fmt.Errorf("init limit reached error logged cache: %w", err)
	}
	s.nodeUsageCache = keyvaluestore.NewJsonFromCollection[NodeUsage](provider.GetSystemCollection())

	uploadingQueueStorage, err := persistentqueue.NewAnystoreStorage(db, "filesync/uploading", makeQueueItem)
	if err != nil {
		return fmt.Errorf("init uploading queue storage: %w", err)
	}
	s.uploadingQueue = persistentqueue.New(uploadingQueueStorage, log.Logger, s.uploadingHandler, queueItemLess, persistentqueue.WithWorkersNumber(5))

	retryUploadingQueueStorage, err := persistentqueue.NewAnystoreStorage(db, "filesync/retry_uploading", makeQueueItem)
	if err != nil {
		return fmt.Errorf("init retry uploading queue storage: %w", err)
	}
	// Retry queue should have the same handler as basic uploading queue
	s.retryUploadingQueue = persistentqueue.New(retryUploadingQueueStorage, log.Logger, s.uploadingHandler, queueItemLess, persistentqueue.WithRetryPause(loopTimeout))

	limitedUploadingQueueStorage, err := persistentqueue.NewAnystoreStorage(db, "filesync/limited_uploading", makeQueueItem)
	if err != nil {
		return fmt.Errorf("init retry uploading queue storage: %w", err)
	}
	s.limitedUploadingQueue = persistentqueue.New(limitedUploadingQueueStorage, log.Logger, s.retryingHandler, queueItemLess, persistentqueue.WithRetryPause(loopTimeout))

	deletionQueueStorage, err := persistentqueue.NewAnystoreStorage(db, "filesync/deletion", makeDeletionQueueItem)
	if err != nil {
		return fmt.Errorf("init deletion queue storage: %w", err)
	}
	s.deletionQueue = persistentqueue.New(deletionQueueStorage, log.Logger, s.deletionHandler, nil)

	retryDeletionQueueStorage, err := persistentqueue.NewAnystoreStorage(db, "filesync/retry_deletion", makeDeletionQueueItem)
	if err != nil {
		return fmt.Errorf("init retry deletion queue storage: %w", err)
	}
	s.retryDeletionQueue = persistentqueue.New(retryDeletionQueueStorage, log.Logger, s.retryDeletionHandler, nil, persistentqueue.WithRetryPause(loopTimeout))

	statusUpdateQueueStorage, err := persistentqueue.NewAnystoreStorage(db, "filesync/status_update", makeStatusUpdateItem)
	if err != nil {
		return fmt.Errorf("init retry deletion queue storage: %w", err)
	}
	s.statusUpdateQueue = persistentqueue.New(statusUpdateQueueStorage, log.Logger, s.statusUpdateHandler, func(one, other *statusUpdateItem) bool {
		return one.Timestamp < other.Timestamp
	})

	s.pendingUploads, err = keyvaluestore.NewJson[*QueueItem](db, "filesync/pending_uploads")
	if err != nil {
		return fmt.Errorf("init limit reached error logged cache: %w", err)
	}

	s.uploadStatusIndex = newUploadStatusIndex(func(fileObjectId string, fullFileId domain.FullFileId, status filesyncstatus.Status) error {
		return s.statusUpdateQueue.Add(&statusUpdateItem{
			FileObjectId: fileObjectId,
			FileId:       fullFileId.FileId.String(),
			SpaceId:      fullFileId.SpaceId,
			Timestamp:    timeid.NewNano(),
			Status:       int(status),
		})
	})
	s.requestsCh = make(chan *fileproto.BlockPushManyRequest, 10)
	s.requestsBatcher = newRequestsBatcher(1024*1024+14, 100*time.Millisecond, s.requestsCh)

	return
}

func (s *fileSync) dagServiceForSpace(spaceID string) ipld.DAGService {
	return filehelper.NewDAGServiceWithSpaceID(spaceID, s.dagService)
}

func (s *fileSync) OnStatusUpdated(callback StatusCallback) {
	s.onStatusUpdated = append(s.onStatusUpdated, callback)
}

func (s *fileSync) Name() (name string) {
	return CName
}

func makeQueueItem() *QueueItem {
	return &QueueItem{}
}

func (s *fileSync) Run(ctx context.Context) (err error) {
	if s.cfg.IsLocalOnlyMode() {
		return
	}

	s.uploadingQueue.Run()
	s.retryUploadingQueue.Run()
	s.limitedUploadingQueue.Run()
	s.deletionQueue.Run()
	s.retryDeletionQueue.Run()
	s.statusUpdateQueue.Run()

	s.closeWg.Add(1)
	go s.runNodeUsageUpdater()
	go s.requestsBatcher.run(s.loopCtx)
	for range 10 {
		go s.runUploader()
	}

	return
}

func (s *fileSync) runUploader() {
	for {
		select {
		case <-s.loopCtx.Done():
			return
		case req := <-s.requestsCh:
			err := s.rpcStore.AddToFileMany(s.loopCtx, req)
			if err != nil {
				log.Error("add to file many:", zap.Error(err))
			} else {
				for _, fb := range req.FileBlocks {
					for _, b := range fb.Blocks {
						c, err := cid.Cast(b.Cid)
						if err != nil {
							log.Error("failed to parse block cid", zap.Error(err))
						} else {
							s.uploadStatusIndex.remove(fb.FileId, c)
						}
					}
				}
			}
			// TODO Retry mechanism
		}
	}
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

	s.closeWg.Wait()

	return nil
}
