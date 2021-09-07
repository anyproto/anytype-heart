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

// threadsDbMigration called on every except the first run of the account
func (s *service) threadsDbMigration(accountThreadId string) error {
	err := s.handleAllMissingDbRecords(accountThreadId)
	if err != nil {
		return fmt.Errorf("handleAllMissingDbRecords failed: %w", err)
	}

	err = s.addMissingThreadsFromCollection()
	if err != nil {
		return fmt.Errorf("addMissingThreadsFromCollection failed: %w", err)
	}

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

	return nil
}

func (s *service) addMissingThreadsFromCollection() error {
	instancesBytes, err := s.threadsCollection.Find(&threadsDb.Query{})
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
	instancesBytes, err := s.threadsCollection.Find(&threadsDb.Query{})
	if err != nil {
		return err
	}

	var threadsInCollection = make(map[string]struct{})
	for _, instanceBytes := range instancesBytes {
		ti := threadInfo{}
		threadsUtil.InstanceFromJSON(instanceBytes, &ti)

		tid, err := thread.Decode(ti.ID.String())
		if err != nil {
			log.Errorf("failed to parse thread id %s: %s", ti.ID, err.Error())
			continue
		}
		threadsInCollection[tid.String()] = struct{}{}
	}

	log.Debugf("%d threads in collection", len(threadsInCollection))

	threadsIds, err := s.logstore.Threads()
	if err != nil {
		return err
	}

	accountId, err := s.derivedThreadIdByIndex(threadDerivedIndexAccount)
	if err != nil {
		return err
	}

	var missingThreads int
	for _, threadId := range threadsIds {
		if threadId.Equals(accountId) {
			continue
		}

		if _, exists := threadsInCollection[threadId.String()]; !exists {
			thrd, err := s.t.GetThread(context.Background(), threadId)
			if err != nil {
				log.Errorf("addMissingThreadsToCollection migration: error getting info: %s\n", err.Error())
				continue
			}
			threadInfo := threadInfo{
				ID:    db.InstanceID(thrd.ID.String()),
				Key:   thrd.Key.String(),
				Addrs: util.MultiAddressesToStrings(thrd.Addrs),
			}

			_, err = s.threadsCollection.Create(threadsUtil.JSONFromInstance(threadInfo))
			if err != nil {
				log.With("thread", thrd.ID.String()).Errorf("failed to create thread at collection: %s: ", err.Error())
			} else {
				missingThreads++
			}
		}
	}

	if missingThreads > 0 {
		log.Warnf("addMissingThreadsToCollection migration: added %d missing threads", missingThreads)
	}

	return nil
}

func (s *service) handleAllMissingDbRecords(threadId string) error {
	tid, err := thread.Decode(threadId)
	if err != nil {
		return fmt.Errorf("failed to parse thread id %s: %s", threadId, err.Error())
	}

	thrd, err := s.t.GetThread(context.Background(), tid)
	if err != nil {
		return fmt.Errorf("failed to get thread info: %s", err.Error())
	}

	for _, logInfo := range thrd.Logs {
		log.Debugf("traversing %s log from head %s", logInfo.ID, logInfo.Head)
		handleAllRecordsInLog(s.db, s.t, thrd, logInfo)
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
