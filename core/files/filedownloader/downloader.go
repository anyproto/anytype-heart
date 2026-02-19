package filedownloader

import (
	"context"
	"fmt"
	"sync"

	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type downloadTask struct {
	objectId string
	spaceId  string
	fileId   domain.FileId
}

type downloader struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	crossSpaceSubService crossspacesub.Service
	objectGetter         cache.ObjectGetter

	eventsQueue *mb.MB[*pb.EventMessage]

	handleTask        func(ctx context.Context, t downloadTask) error
	filesSubscription *objectsubscription.ObjectSubscription[downloadTask]

	requestTaskCh   chan chan downloadTask
	addTaskCh       chan downloadTask
	removeTaskCh    chan string
	waitingRequests []chan downloadTask

	lock  sync.Mutex
	tasks map[string]downloadTask
}

func (s *service) newDownloader() *downloader {
	ctx, ctxCancel := context.WithCancel(s.ctx)
	return &downloader{
		ctx:                  ctx,
		ctxCancel:            ctxCancel,
		crossSpaceSubService: s.crossSpaceSubService,
		objectGetter:         s.objectGetter,
		handleTask: func(ctx context.Context, t downloadTask) error {
			return s.DownloadToLocalStore(ctx, t.spaceId, t.fileId, 0)
		},
		requestTaskCh: make(chan chan downloadTask),
		addTaskCh:     make(chan downloadTask),
		removeTaskCh:  make(chan string),
		lock:          sync.Mutex{},
		tasks:         map[string]downloadTask{},
	}
}

func (s *downloader) close() {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.eventsQueue != nil {
		err := s.eventsQueue.Close()
		if err != nil {
			log.Error("close events queue", zap.Error(err))
		}
	}
	if s.filesSubscription != nil {
		s.filesSubscription.Close()
	}
}

func (s *downloader) stop() {
	s.ctxCancel()
}

func (s *downloader) start() {
	for range 5 {
		go s.runDownloadWorker()
	}

	go func() {
		err := s.runSubscription()
		if err != nil {
			log.Error("run subscription", zap.Error(err))
		}
	}()

	go s.runManager()
}

func (s *downloader) runSubscription() error {
	s.lock.Lock()
	s.eventsQueue = mb.New[*pb.EventMessage](0)
	s.lock.Unlock()

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
			bundle.RelationKeySpaceId.String(),
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
		select {
		case <-s.ctx.Done():
			return nil
		case s.addTaskCh <- task:
		}
	}

	s.lock.Lock()
	s.filesSubscription = objectsubscription.NewFromQueue(s.eventsQueue, objectsubscription.SubscriptionParams[downloadTask]{
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
	}, nil)
	s.lock.Unlock()

	err = s.filesSubscription.Run()
	if err != nil {
		return fmt.Errorf("run subscription: %w", err)
	}

	return nil
}

func (s *downloader) runManager() {
	for {
		select {
		case <-s.ctx.Done():
			s.close()
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

func (s *downloader) runDownloadWorker() {
	for {
		task, ok := s.getNextTask()
		if !ok {
			return
		}
		err := s.handleTask(s.ctx, task)
		if err != nil {
			log.Error("auto download file", zap.String("objectId", task.objectId), zap.Error(err))
			continue
		}
		err = cache.Do(s.objectGetter, task.objectId, func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			localDetails := st.LocalDetails()
			localDetails.SetBool(bundle.RelationKeyFileAvailableOffline, true)
			return sb.Apply(st)
		})
		if err != nil {
			log.Error("mark file as available offline", zap.String("objectId", task.objectId), zap.Error(err))
		}
	}
}

func (s *downloader) getNextTask() (downloadTask, bool) {
	getTaskCh := make(chan downloadTask, 1)

	select {
	case <-s.ctx.Done():
		return downloadTask{}, false
	case s.requestTaskCh <- getTaskCh:
	}

	select {
	case <-s.ctx.Done():
		return downloadTask{}, false
	case task := <-getTaskCh:
		return task, true
	}
}
