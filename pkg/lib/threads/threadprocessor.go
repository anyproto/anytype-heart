package threads

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/textileio/go-threads/core/logstore"
	"github.com/textileio/go-threads/core/thread"
	threadsDb "github.com/textileio/go-threads/db"
	threadsUtil "github.com/textileio/go-threads/util"
)

type ThreadProcessor interface {
	Init(thread.ID) error
	Listen(map[thread.ID]threadInfo) error
	GetCollection() *threadsDb.Collection
	GetDB() *threadsDb.DB
}

type threadProcessor struct {
	threadsService *service
	threadNotifier ThreadDownloadNotifier

	db                *threadsDb.DB
	threadsCollection *threadsDb.Collection

	isAccountProcessor bool

	threadId thread.ID
}

func (t *threadProcessor) GetCollection() *threadsDb.Collection {
	return t.threadsCollection
}

func (t *threadProcessor) GetDB() *threadsDb.DB {
	return t.db
}

func NewThreadProcessor(s *service, notifier ThreadDownloadNotifier) ThreadProcessor {
	return &threadProcessor{
		threadsService: s,
		threadNotifier: notifier,
	}
}

func NewAccountThreadProcessor(s *service, simultaneousRequests int) ThreadProcessor {
	return &threadProcessor{
		threadsService:     s,
		isAccountProcessor: true,
		threadNotifier:     NewAccountNotifier(simultaneousRequests),
	}
}

func (t *threadProcessor) Init(id thread.ID) error {
	if t.db != nil {
		return nil
	}

	if id == thread.Undef {
		return fmt.Errorf("cannot start processor with undefined thread")
	}
	t.threadId = id

	tInfo, err := t.threadsService.t.GetThread(context.Background(), id)
	if err != nil {
		return fmt.Errorf("cannot start thread processor, because thread is not downloaded: %w", err)
	}

	t.db, err = threadsDb.NewDB(
		context.Background(),
		t.threadsService.threadsDbDS,
		t.threadsService.t,
		t.threadId,
		// We need to provide the key beforehand
		// otherwise there can be problems if the log is not created (and therefore the keys are not matched)
		// this happens with workspaces, because we are adding threads but not creating them
		threadsDb.WithNewKey(tInfo.Key),
		threadsDb.WithNewCollections())
	if err != nil {
		return err
	}

	threadIdString := t.threadId.String()
	// To not break the old behaviour we call account thread collection with the same name we used before
	if t.isAccountProcessor {
		threadIdString = ""
	}
	collectionName := fmt.Sprintf("%s%s", ThreadInfoCollectionName, threadIdString)
	t.threadsCollection = t.db.GetCollection(collectionName)

	if t.threadsCollection == nil {
		collectionConfig := threadsDb.CollectionConfig{
			Name:   collectionName,
			Schema: threadsUtil.SchemaFromInstance(threadInfo{}, false),
		}
		t.threadsCollection, err = t.db.NewCollection(collectionConfig)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *threadProcessor) Listen(initialThreads map[thread.ID]threadInfo) error {
	WorkspaceLogger.
		With("is account", t.isAccountProcessor).
		With("workspace id", t.threadId).
		Info("started listening for workspace")

	log.With("thread id", t.threadId).
		Info("listen for workspace")
	l, err := t.db.Listen()
	if err != nil {
		return err
	}

	threadsTotal := len(initialThreads)
	initialThreadsLock := sync.RWMutex{}

	removeElement := func(tid thread.ID) {
		log.With("thread id", tid.String()).
			Debug("removing thread from processing")
		initialThreadsLock.RLock()
		_, isInitialThread := initialThreads[tid]
		initialThreadsLock.RUnlock()

		if isInitialThread {
			initialThreadsLock.Lock()
			delete(initialThreads, tid)
			threadsTotal--
			t.threadNotifier.SetTotalThreads(threadsTotal)
			initialThreadsLock.Unlock()
		}
	}

	processThread := func(tid thread.ID, ti threadInfo) {
		log.With("thread id", tid.String()).
			Debugf("trying to process new thread")
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		info, err := t.threadsService.t.GetThread(ctx, tid)
		cancel()
		if err != nil && err != logstore.ErrThreadNotFound {
			log.With("thread", tid.String()).
				Errorf("error getting thread while processing: %v", err)
			removeElement(tid)
			return
		}
		if info.ID != thread.Undef {
			// our own event
			removeElement(tid)
			return
		}

		metrics.ExternalThreadReceivedCounter.Inc()
		go func() {
			if err := t.threadsService.processNewExternalThreadUntilSuccess(tid, ti); err != nil {
				log.With("thread", tid.String()).Error("processNewExternalThreadUntilSuccess failed: %t", err.Error())
				return
			}

			ch := t.threadsService.getNewThreadChan()
			initialThreadsLock.RLock()
			if len(initialThreads) == 0 {
				initialThreadsLock.RUnlock()
			} else {
				_, isInitialThread := initialThreads[tid]
				initialThreadsLock.RUnlock()
				if isInitialThread {
					initialThreadsLock.Lock()

					delete(initialThreads, tid)
					t.threadNotifier.AddThread()
					if len(initialThreads) == 0 {
						t.threadNotifier.Finish()
					}

					initialThreadsLock.Unlock()
				}
			}
			if ch != nil && !t.threadsService.stopped {
				select {
				case <-t.threadsService.ctx.Done():
				case ch <- tid.String():
				}
			}
		}()
	}

	processThreadActions := func(actions []threadsDb.Action) {
		for _, action := range actions {
			// TODO: add thread delete actions, consider moving create logic to another function
			if action.Type != threadsDb.ActionCreate {
				continue
			}
			instanceBytes, err := t.threadsCollection.FindByID(action.ID)
			if err != nil {
				log.Errorf("failed to find thread info for id %s: %v", action.ID.String(), err)
				continue
			}

			ti := threadInfo{}
			threadsUtil.InstanceFromJSON(instanceBytes, &ti)
			tid, err := thread.Decode(ti.ID.String())
			if err != nil {
				log.Errorf("failed to parse thread id %s: %v", ti.ID.String(), err)
				continue
			}
			initialThreadsLock.RLock()
			if len(initialThreads) != 0 {
				_, ok := initialThreads[tid]
				// if we are already downloading this thread as initial one
				if ok {
					initialThreadsLock.RUnlock()
					continue
				}
			}
			initialThreadsLock.RUnlock()
			processThread(tid, ti)
		}
	}

	if threadsTotal != 0 {
		log.With("thread count", threadsTotal).
			Info("pulling initial threads")

		if os.Getenv("ANYTYPE_RECOVERY_PROGRESS") == "1" {
			log.Info("adding progress bar")
			t.threadNotifier.Start(t.threadsService.process)
		}
		t.threadNotifier.SetTotalThreads(threadsTotal)

		initialMapCopy := make(map[thread.ID]threadInfo)
		for tid, ti := range initialThreads {
			initialMapCopy[tid] = ti
		}

		// processing all initial threads if any
		go func() {
			for tid, ti := range initialMapCopy {
				log.With("thread id", tid.String()).
					Debugf("going to process initial thread")
				processThread(tid, ti)
			}
		}()
	}

	go func() {
		defer func() {
			l.Close()
			t.threadsService.closeThreadChan()
		}()
		deadline := 1 * time.Second
		tmr := time.NewTimer(deadline)
		flushBuffer := make([]threadsDb.Action, 0, 100)
		timerRead := false

		processBuffer := func() {
			if len(flushBuffer) == 0 {
				return
			}
			buffCopy := make([]threadsDb.Action, len(flushBuffer))
			for index, action := range flushBuffer {
				buffCopy[index] = action
			}
			flushBuffer = flushBuffer[:0]
			go processThreadActions(buffCopy)
		}

		for {
			select {
			case <-t.threadsService.ctx.Done():
				processBuffer()
				return
			case _ = <-tmr.C:
				timerRead = true
				// we don't have new messages for at least deadline and we have something to flush
				processBuffer()

			case c := <-l.Channel():
				log.With("thread id", c.ID.String()).
					Debugf("received new thread through channel")
				// as per docs the timer should be stopped or expired with drained channel
				// to be reset
				if !tmr.Stop() && !timerRead {
					<-tmr.C
				}
				tmr.Reset(deadline)
				timerRead = false
				flushBuffer = append(flushBuffer, c)
			}
		}
	}()

	return nil
}
