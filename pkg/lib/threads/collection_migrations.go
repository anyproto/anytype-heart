package threads

import (
	"context"
	"errors"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	threadsApp "github.com/textileio/go-threads/core/app"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/textileio/go-threads/cbor"
	"github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/logstore"
	threadsNet "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	threadsDb "github.com/textileio/go-threads/db"
	threadsUtil "github.com/textileio/go-threads/util"
)

// handleMissingRecordsForCollection called on every except the first run of the account
func (s *service) handleMissingRecordsForCollection(threadId string, db *threadsDb.DB, collection *threadsDb.Collection) error {
	err := s.handleAllMissingDbRecords(threadId, db)
	if err != nil {
		return fmt.Errorf("handleAllMissingDbRecords failed: %w", err)
	}

	err = s.addMissingThreadsFromCollection(collection)
	if err != nil {
		return fmt.Errorf("addMissingThreadsFromCollection failed: %w", err)
	}

	return nil
}

func (s *service) handleMissingReplicatorsAndThreadsInQueue() {
	go func() {
		err := s.addMissingReplicators()
		if err != nil {
			log.Errorf("addMissingReplicators: %s", err.Error())
		}
	}()

	go func() {
		err := s.addMissingThreadsToCollection()
		if err != nil {
			log.Errorf("addMissingThreadsToCollection: %s", err.Error())
		}
	}()
}

func (s *service) addMissingThreadsFromCollection(collection *threadsDb.Collection) error {
	instancesBytes, err := collection.Find(&threadsDb.Query{})
	if err != nil {
		return err
	}

	var missingThreadsAdded int
	for _, instanceBytes := range instancesBytes {
		ti := threadInfo{}
		threadsUtil.InstanceFromJSON(instanceBytes, &ti)

		tid, err := thread.Decode(ti.ID.String())
		if err != nil {
			log.Errorf("failed to parse thread id %s: %s", ti.ID, err.Error())
			continue
		}

		if _, err = s.t.GetThread(context.Background(), tid); err != nil && errors.Is(err, logstore.ErrThreadNotFound) {
			metrics.ExternalThreadReceivedCounter.Add(1)
			missingThreadsAdded++
			go func() {
				if s.processNewExternalThreadUntilSuccess(tid, ti) != nil {
					log.With("thread", tid.String()).Error("processNewExternalThreadUntilSuccess failed: %s", err.Error())
					return
				}

				// here we need to lock any changes to the channel before we send to it
				// otherwise we may receive panic
				s.Lock()
				defer s.Unlock()
				if s.newThreadChan != nil {
					select {
					case <-s.ctx.Done():
					case s.newThreadChan <- tid.String():
					}
				}
			}()
		}
	}

	if missingThreadsAdded > 0 {
		log.Warnf("addMissingThreadsFromCollection: processing %d missing threads in background...", missingThreadsAdded)
	}
	return nil
}

func (s *service) addMissingThreadsToCollection() error {
	unprocessedEntries, err := s.threadCreateQueue.GetAllQueueEntries()
	if err != nil {
		return fmt.Errorf("could not get entries from queue: %w", err)
	}

	for _, entry := range unprocessedEntries {
		threadId, err := thread.Decode(entry.ThreadId)
		if err != nil {
			log.With("thread id", entry.ThreadId).
				Errorf("add missing threads to collection: could not decode thread with err: %v", err)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		thrdInfo, err := s.t.GetThread(ctx, threadId)
		cancel()

		if err != nil {
			// The thread was not created so there is no point in creating it and adding to collection
			if err == logstore.ErrThreadNotFound {
				_ = s.threadCreateQueue.RemoveThreadQueueEntry(entry.ThreadId)
			} else {
				log.With("thread id", entry.ThreadId).
					Errorf("add missing threads to collection: could not get thread with err: %v", err)
			}
			continue
		}
		threadInfo := threadInfo{
			ID:    db.InstanceID(thrdInfo.ID.String()),
			Key:   thrdInfo.Key.String(),
			Addrs: util.MultiAddressesToStrings(thrdInfo.Addrs),
		}
		collectionThreadId, err := thread.Decode(entry.CollectionThread)
		if err != nil {
			log.With("thread id", entry.ThreadId).
				With("collection thread", entry.CollectionThread).
				Errorf("add missing threads to collection: could not decode collection thread: %v", err)
			continue
		}

		s.processorMutex.RLock()
		processor, exists := s.threadProcessors[collectionThreadId]
		s.processorMutex.RUnlock()

		// we didn't spin the collection so far
		if !exists {
			// trying to spin the collection
			processor, err = s.startWorkspaceThreadProcessor(collectionThreadId.String())
			if err != nil {
				log.With("thread id", entry.ThreadId).
					With("collection thread", entry.CollectionThread).
					Errorf("add missing threads to collection: could not start thread processor: %v", err)
				continue
			}
		}
		collection := processor.GetCollection()

		_, err = collection.FindByID(threadInfo.ID)
		if err == nil {
			_ = s.threadCreateQueue.RemoveThreadQueueEntry(threadInfo.ID.String())
			continue
		}

		WorkspaceLogger.
			With("collection name", collection.GetName()).
			Info("adding missing thread to collection")
		_, err = collection.Create(threadsUtil.JSONFromInstance(threadInfo))
		if err == nil {
			_ = s.threadCreateQueue.RemoveThreadQueueEntry(threadInfo.ID.String())
		}
	}

	return nil
}

func (s *service) handleAllMissingDbRecords(threadId string, db *threadsDb.DB) error {
	tid, err := thread.Decode(threadId)
	if err != nil {
		return fmt.Errorf("failed to parse thread id %s: %s", threadId, err.Error())
	}

	thrd, err := s.t.GetThread(context.Background(), tid)
	if err != nil {
		return fmt.Errorf("failed to get thread info: %s", err.Error())
	}

	for _, logInfo := range thrd.Logs {
		log.Debugf("traversing %s log from head %s(%d)", logInfo.ID, logInfo.Head.ID, logInfo.Head.Counter)
		handleAllRecordsInLog(db, s.t, thrd, logInfo)
	}
	return nil
}

func handleAllRecordsInLog(tdb *threadsDb.DB, net threadsApp.Net, thrd thread.Info, li thread.LogInfo) {
	var (
		rid     = li.Head.ID
		total   = 0
		records []threadRecord
	)

	handled := 0
	defer func() {
		for i := len(records) - 1; i >= 0; i-- {
			ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
			err := tdb.HandleNetRecord(ctx, records[i], thrd.Key)
			if err != nil {
				// todo: errCantCreateExistingInstance error is not exported and has a typo
				if !strings.HasSuffix(err.Error(), "already existent instance") {
					log.Errorf("failed to handle record: %s", err.Error())
				}
			} else {
				handled++
			}
		}
		if handled > 0 {
			log.Warnf("handleAllRecordsInLog: handled %d missing records", handled)
		}
	}()

	for {
		if !rid.Defined() {
			return
		}
		total++
		rec, _, _, err := getRecord(net, thrd, rid)
		if rec != nil {
			trec := threadRecord{
				Record:   rec,
				threadID: thrd.ID,
				logID:    li.ID,
			}

			records = append(records, trec)
			rid = rec.PrevID()
		} else {
			log.Errorf("can't continue the traverse, failed to load a record: %s", err.Error())
			return
		}
	}
}

func getRecord(net threadsApp.Net, thrd thread.Info, rid cid.Cid) (threadsNet.Record, *cbor.Event, format.Node, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if thrd.ID == thread.Undef {
		return nil, nil, nil, fmt.Errorf("undef id")
	}

	rec, err := net.GetRecord(ctx, thrd.ID, rid)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load record: %s", err.Error())
	}

	event, err := cbor.EventFromRecord(ctx, net, rec)
	if err != nil {
		return rec, nil, nil, fmt.Errorf("failed to load event: %s", err.Error())
	}

	node, err := event.GetBody(context.TODO(), net, thrd.Key.Read())
	if err != nil {
		return rec, event, nil, fmt.Errorf("failed to get record body: %w", err)
	}

	return rec, event, node, nil
}
