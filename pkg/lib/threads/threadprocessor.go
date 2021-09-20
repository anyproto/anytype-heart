package threads

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"os"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/textileio/go-threads/core/db"
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

func (t *threadProcessor) Init(id thread.ID) error {
	if t.db != nil {
		return nil
	}

	if id == thread.Undef {
		return fmt.Errorf("cannot start db with undefined thread")
	}
	t.threadId = id

	var err error
	t.db, err = threadsDb.NewDB(
		context.Background(),
		t.threadsService.threadsDbDS,
		t.threadsService.t,
		t.threadId,
		threadsDb.WithNewCollections())
	if err != nil {
		return err
	}

	t.threadsCollection = t.db.GetCollection(ThreadInfoCollectionName)

	if t.threadsCollection == nil {
		t.threadsCollection, err = t.db.NewCollection(threadInfoCollection)
		if err != nil {
			return err
		}
	}

	return nil
}

func (t *threadProcessor) Listen(initialThreads map[thread.ID]threadInfo) error {
	log.With("thread id", t.threadId).
		Info("threadsDbListen for workspace")
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
			smartBlockType, err := smartblock.SmartBlockTypeFromThreadID(tid)
			if err == nil && smartBlockType == smartblock.SmartBlockTypeWorkspace {
				err = t.addNewProcessor(tid)
				if err != nil {
					log.Errorf("could not add new processor: %v", err)
				}
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

	processThreads := func(ids []db.InstanceID) {
		for _, id := range ids {
			instanceBytes, err := t.threadsCollection.FindByID(id)
			if err != nil {
				log.Errorf("failed to find thread info for id %t: %w", id.String(), err)
				continue
			}

			ti := threadInfo{}
			threadsUtil.InstanceFromJSON(instanceBytes, &ti)
			tid, err := thread.Decode(ti.ID.String())
			if err != nil {
				log.Errorf("failed to parse thread id %t: %t", ti.ID, err.Error())
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
		flushBuffer := make([]db.InstanceID, 0, 100)
		timerRead := false

		processBuffer := func() {
			if len(flushBuffer) == 0 {
				return
			}
			buffCopy := make([]db.InstanceID, len(flushBuffer))
			for index, id := range flushBuffer {
				buffCopy[index] = id
			}
			flushBuffer = flushBuffer[:0]
			go processThreads(buffCopy)
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
				flushBuffer = append(flushBuffer, c.ID)
			}
		}
	}()

	return nil
}

func (t *threadProcessor) addNewProcessor(threadId thread.ID) error {
	t.threadsService.processorMutex.RLock()
	_, exists := t.threadsService.threadProcessors[threadId]
	t.threadsService.processorMutex.RUnlock()
	if exists {
		return fmt.Errorf("thread processor with id %s already exists", threadId.String())
	}

	newProcessor := NewThreadProcessor(t.threadsService, NewNoOpNotifier())
	err := newProcessor.Init(threadId)
	if err != nil {
		return fmt.Errorf("could not initialize new thread processor %w", err)
	}
	t.threadsService.processorMutex.Lock()
	defer t.threadsService.processorMutex.Unlock()
	t.threadsService.threadProcessors[threadId] = newProcessor
	return nil
}
