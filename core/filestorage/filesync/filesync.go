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

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/core/filestorage/rpcstore"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/space/spacecore/spacecore"
)

const CName = "filesync"

var log = logger.NewNamed(CName)

var loopTimeout = time.Minute

var errReachedLimit = fmt.Errorf("file upload limit has been reached")

type FileSync interface {
	AddFile(spaceID, fileID string, uploadedByUser, imported bool) (err error)
	OnUpload(func(spaceID, fileID string) error)
	RemoveFile(spaceId, fileId string) (err error)
	SpaceStat(ctx context.Context, spaceId string) (ss SpaceStat, err error)
	FileStat(ctx context.Context, spaceId, fileId string) (fs FileStat, err error)
	FileListStats(ctx context.Context, spaceId string, fileIDs []string) ([]FileStat, error)
	SyncStatus() (ss SyncStatus, err error)
	HasUpload(spaceId, fileId string) (ok bool, err error)
	IsFileUploadLimited(spaceId, fileId string) (ok bool, err error)
	DebugQueue(*http.Request) (*QueueInfo, error)
	SendImportEvents()
	ClearImportEvents()
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
	dbProvider   datastore.Datastore
	rpcStore     rpcstore.RpcStore
	queue        *fileSyncStore
	loopCtx      context.Context
	loopCancel   context.CancelFunc
	uploadPingCh chan struct{}
	removePingCh chan struct{}
	dagService   ipld.DAGService
	fileStore    filestore.FileStore
	eventSender  event.Sender
	onUpload     func(spaceID, fileID string) error
	spaceService spacecore.SpaceService

	spaceStatsLock    sync.Mutex
	spaceStats        map[string]SpaceStat
	importEventsMutex sync.Mutex
	importEvents      []*pb.Event
}

func New() FileSync {
	return &fileSync{
		spaceStats: map[string]SpaceStat{},
	}
}

func (f *fileSync) Init(a *app.App) (err error) {
	f.dbProvider = a.MustComponent(datastore.CName).(datastore.Datastore)
	f.rpcStore = a.MustComponent(rpcstore.CName).(rpcstore.Service).NewStore()
	f.dagService = a.MustComponent(fileservice.CName).(fileservice.FileService).DAGService()
	f.fileStore = app.MustComponent[filestore.FileStore](a)
	f.spaceService = app.MustComponent[spacecore.SpaceService](a)
	f.eventSender = app.MustComponent[event.Sender](a)
	f.removePingCh = make(chan struct{})
	f.uploadPingCh = make(chan struct{})
	return
}

func (s *fileSync) dagServiceForSpace(spaceID string) ipld.DAGService {
	return filehelper.NewDAGServiceWithSpaceID(spaceID, s.dagService)
}

func (f *fileSync) OnUpload(callback func(spaceID, fileID string) error) {
	f.onUpload = callback
}

func (f *fileSync) Name() (name string) {
	return CName
}

func (f *fileSync) Run(ctx context.Context) (err error) {
	db, err := f.dbProvider.SpaceStorage()
	if err != nil {
		return
	}
	f.queue, err = newFileSyncStore(db)
	if err != nil {
		return
	}

	go f.precacheSpaceStats()

	f.loopCtx, f.loopCancel = context.WithCancel(context.Background())
	go f.addLoop()
	go f.removeLoop()
	return
}

func (f *fileSync) precacheSpaceStats() {
	// TODO multi-spaces: init for each space: GO-1681
	// TODO: [MR] adapt to multi-spaces
	spaceID := f.spaceService.AccountId()
	_, err := f.SpaceStat(context.Background(), spaceID)
	if err != nil {
		// Don't confuse users with 0B limit in case of error, so set default 1GB limit
		f.setSpaceStats(spaceID, SpaceStat{
			SpaceId:    spaceID,
			BytesLimit: 1024 * 1024 * 1024, // 1 GB
		})
		log.Error("can't init space stats", zap.String("spaceID", f.spaceService.AccountId()), zap.Error(err))
	}
}

func (f *fileSync) Close(ctx context.Context) (err error) {
	if f.loopCancel != nil {
		f.loopCancel()
	}
	if closer, ok := f.rpcStore.(io.Closer); ok {
		if err = closer.Close(); err != nil {
			log.Error("can't close rpc store", zap.Error(err))
		}
	}
	return nil
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
