package filedownloader

import (
	"context"
	"fmt"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "core.files.filedownloader"

var log = logging.Logger(CName).Desugar()

type Service interface {
	SetEnabled(enabled bool)
	app.ComponentRunnable
}

type service struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	dbProvider           anystoreprovider.Provider
	downloaded           anystore.Collection
	crossSpaceSubService crossspacesub.Service

	eventsQueue *mb.MB[*pb.EventMessage]

	filesSubscription *objectsubscription.ObjectSubscription[downloadTask]

	requestTaskCh   chan chan downloadTask
	addTaskCh       chan downloadTask
	removeTaskCh    chan string
	waitingRequests []chan downloadTask

	lock    sync.Mutex
	enabled bool
	tasks   map[string]downloadTask
}

func New() Service {
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &service{
		ctx:           ctx,
		ctxCancel:     ctxCancel,
		requestTaskCh: make(chan chan downloadTask),
		addTaskCh:     make(chan downloadTask),
		removeTaskCh:  make(chan string),
		lock:          sync.Mutex{},
		enabled:       false,
		tasks:         map[string]downloadTask{},
	}
}

type downloadTask struct {
	objectId string
	spaceId  string
	fileId   domain.FileId
}

func (s *service) SetEnabled(enabled bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.enabled = enabled
}

func (s *service) Init(a *app.App) error {
	s.dbProvider = app.MustComponent[anystoreprovider.Provider](a)
	s.crossSpaceSubService = app.MustComponent[crossspacesub.Service](a)
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) Run(ctx context.Context) error {

	go s.runManager()
	for range 5 {
		go s.runDownloadWorker()
	}

	err := s.runSubscription()
	if err != nil {
		return fmt.Errorf("run subscription: %w", err)
	}

	return nil
}

func (s *service) runSubscription() error {
	s.eventsQueue = mb.New[*pb.EventMessage](0)
	resp, err := s.crossSpaceSubService.Subscribe(subscription.SubscribeRequest{
		SubId:         CName,
		InternalQueue: s.eventsQueue,
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyFileAvailableOffline,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(false),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List(domain.FileLayouts),
			},
			{
				RelationKey: bundle.RelationKeySizeInBytes,
				Condition:   model.BlockContentDataviewFilter_Less,
				Value:       domain.Int64(20 * 1024 * 1024),
			},
			{
				RelationKey: bundle.RelationKeyFileBackupStatus,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(filesyncstatus.Synced),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyFileId.String(),
		},
		NoDepSubscription: true,
	}, crossspacesub.NoOpPredicate())
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	detailsToTask := func(details *domain.Details) (string, downloadTask) {
		id := details.GetString(bundle.RelationKeyId)
		spaceId := details.GetString(bundle.RelationKeySpaceId)
		fileId := domain.FileId(details.GetString(bundle.RelationKeyFileId))
		return id, downloadTask{
			objectId: id,
			spaceId:  spaceId,
			fileId:   fileId,
		}
	}

	for _, rec := range resp.Records {
		_, task := detailsToTask(rec)
		s.addTaskCh <- task
	}

	objSub := objectsubscription.NewFromQueue(s.eventsQueue, objectsubscription.SubscriptionParams[downloadTask]{
		SetDetails: detailsToTask,
		UpdateKeys: func(keyValues []objectsubscription.RelationKeyValue, curEntry downloadTask) (updatedEntry downloadTask) {
			return curEntry
		},
		RemoveKeys: func(keys []string, curEntry downloadTask) (updatedEntry downloadTask) {
			return curEntry
		},
		OnAdded: func(id string, entry downloadTask) {
			s.addTaskCh <- entry
		},
		OnRemoved: func(id string, entry downloadTask) {
			s.removeTaskCh <- id
		},
	})

	err = objSub.Run()
	if err != nil {
		return fmt.Errorf("run subscription: %w", err)
	}

	return nil
}

func (s *service) runManager() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case req := <-s.requestTaskCh:
			if len(s.tasks) > 0 {
				// Take any task
				for _, task := range s.tasks {
					delete(s.tasks, task.objectId)
					req <- task
					break
				}
			} else {
				s.waitingRequests = append(s.waitingRequests, req)
			}
		case add := <-s.addTaskCh:
			if len(s.waitingRequests) > 0 {
				req := s.waitingRequests[len(s.waitingRequests)-1]
				s.waitingRequests = s.waitingRequests[:len(s.waitingRequests)-1]
				req <- add
			} else {
				s.tasks[add.objectId] = add
			}
		case id := <-s.removeTaskCh:
			delete(s.tasks, id)
		}
	}
}

func (s *service) runDownloadWorker() {
	for {
		task, ok := s.getNextTask()
		if !ok {
			return
		}
		fmt.Println("downloading", task.objectId)
	}
}

func (s *service) getNextTask() (downloadTask, bool) {
	getTaskCh := make(chan downloadTask, 1)

	s.requestTaskCh <- getTaskCh

	select {
	case <-s.ctx.Done():
		return downloadTask{}, false
	case task := <-getTaskCh:
		return task, true
	}
}

func (s *service) Close(ctx context.Context) error {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	err := s.eventsQueue.Close()
	if err != nil {
		log.Error("close events queue", zap.Error(err))
	}
	return nil
}
