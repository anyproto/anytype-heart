package filesync

/*
AI generated

Name: File Sync to Backup Node
Scope: global

## Responsibility
- Upload files (IPLD DAG blocks) to the backup node (file node)
- Delete files from the backup node
- Track and enforce storage limits per space and per account
- Provide node/space usage statistics

## Background Tasks
- nodeUsageUpdater: periodically fetches account usage from backup node (10s active, 1min idle) [runNodeUsageUpdater]
- uploader (x10): processes pending uploads from queue [runUploader]
- batchUploader (x10): sends batched block upload requests to backup node [runBatchUploader]
- requestsBatcher: batches small files together, splits large files across requests [requestsBatcher.run]
- limitedUploader: retries uploads when space becomes available [runLimitedUploader]
- deleter: processes pending deletions from queue [runDeleter]
- spaceUsage (per space): periodically updates per-space limits from backup node [spaceUsage.update]

## External State
- filesync/queue collection: persistent queue of file upload/delete operations

## Documentation
File states: PendingUpload -> Uploading -> Done (or Limited if quota exceeded)
                          -> PendingDeletion -> Deleted

Upload flow:
1. AddFile enqueues file with PendingUpload state
2. Uploader checks block availability on node (which blocks need upload vs bind)
3. Allocates space in local limit tracker
4. Walks DAG and batches blocks via requestsBatcher
5. batchUploader sends BlockPushMany requests to node
6. On completion, marks file as Done; on limit error, marks as Limited

Limited files are retried when spaceUsageManager detects more free space available.
*/

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

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	rpcstore2 "github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/core/files/filesync/filequeue"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
)

const CName = "filesync"

var log = logger.NewNamed(CName)

var loopTimeout = time.Minute

type StatusCallback func(fileObjectId string, fileId domain.FullFileId, status filesyncstatus.Status) error

type FileSync interface {
	AddFile(req AddFileRequest) (err error)
	OnStatusUpdated(StatusCallback)
	DeleteFile(objectId string, fileId domain.FullFileId) (err error)
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

	nodeUsageCache keyvaluestore.Store[NodeUsage]

	limitManager    *spaceUsageManager
	requestsBatcher *requestsBatcher
	requestsCh      chan blockPushManyRequest

	queue *filequeue.Queue[FileInfo]

	importEventsMutex sync.Mutex
	importEvents      []*pb.Event
	cfg               *config.Config

	closeWg *sync.WaitGroup
}

type spaceService interface {
	TechSpaceId() string
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
	techSpaceId := app.MustComponent[spaceService](a).TechSpaceId()
	s.limitManager = newSpaceUsageManager(app.MustComponent[subscription.Service](a), s.rpcStore, techSpaceId)
	err = s.limitManager.init()
	if err != nil {
		return fmt.Errorf("init limitManager: %w", err)
	}

	provider := app.MustComponent[anystoreprovider.Provider](a)
	db := provider.GetCommonDb()

	queueColl, err := db.Collection(context.Background(), "filesync/queue")
	if err != nil {
		return fmt.Errorf("get queue collection: %w", err)
	}

	store := filequeue.NewStorage(queueColl, marshalFileInfo, unmarshalFileInfo)
	s.queue = filequeue.NewQueue(store, func(info FileInfo) string {
		return info.ObjectId
	}, func(info FileInfo, id string) FileInfo {
		info.ObjectId = id
		return info
	})

	s.nodeUsageCache = keyvaluestore.NewJsonFromCollection[NodeUsage](provider.GetSystemCollection())

	s.requestsCh = make(chan blockPushManyRequest, 10)
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

func (s *fileSync) Run(ctx context.Context) error {
	if s.cfg.IsLocalOnlyMode() {
		return nil
	}

	go func() {
		s.queue.Run()
	}()

	s.closeWg.Add(1)
	go s.runNodeUsageUpdater()
	go s.requestsBatcher.run(s.loopCtx)
	for range 10 {
		go s.runBatchUploader()
	}

	for {
		err := s.resetUploadingStatus(ctx)
		if errors.Is(err, filequeue.ErrNoRows) {
			break
		}
		if err != nil {
			log.Error("reset uploading status", zap.Error(err))
		}
	}

	for range 10 {
		go s.runUploader(s.loopCtx)
	}

	go s.runLimitedUploader(s.loopCtx)

	go s.runDeleter()

	return nil
}

func (s *fileSync) Close(ctx context.Context) error {
	if s.loopCancel != nil {
		s.loopCancel()
	}
	s.queue.Close()
	// Don't wait
	go func() {
		if closer, ok := s.rpcStore.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				log.Error("can't close rpc store", zap.Error(err))
			}
		}
	}()

	s.limitManager.close()
	s.closeWg.Wait()

	return nil
}
