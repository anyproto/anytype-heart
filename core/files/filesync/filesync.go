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

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	rpcstore2 "github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore"
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
	UploadSynchronously(ctx context.Context, spaceId string, fileId domain.FileId) error
	OnStatusUpdated(StatusCallback)
	CancelDeletion(objectId string, fileId domain.FullFileId) (err error)
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

	limitManager    *uploadLimitManager
	requestsBatcher *requestsBatcher
	requestsCh      chan blockPushManyRequest

	filesRepository *filesRepository
	stateProcessor  *stateProcessor

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

	s.filesRepository = newFilesRepository()
	s.stateProcessor = newStateProcessor(s.filesRepository)

	provider := app.MustComponent[anystoreprovider.Provider](a)
	db := provider.GetCommonDb()
	_ = db

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

func (s *fileSync) Run(ctx context.Context) (err error) {
	if s.cfg.IsLocalOnlyMode() {
		return
	}

	s.closeWg.Add(1)
	go s.runNodeUsageUpdater()
	go s.requestsBatcher.run(s.loopCtx)
	for range 10 {
		go s.runBatchUploader()
	}

	go s.runUploader(s.loopCtx)

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

	s.closeWg.Wait()

	return nil
}
