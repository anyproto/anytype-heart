package threads

import (
	"context"
	"fmt"
	"strings"
	"time"

	format "github.com/ipfs/go-ipld-format"
	"github.com/libp2p/go-libp2p-core/peer"
	threadsNet "github.com/textileio/go-threads/core/net"

	"github.com/ipfs/go-cid"
	"github.com/textileio/go-threads/cbor"
	threadsApp "github.com/textileio/go-threads/core/app"
	"github.com/textileio/go-threads/core/thread"
	threadsDb "github.com/textileio/go-threads/db"
)

type threadRecord struct {
	threadsNet.Record
	threadID thread.ID
	logID    peer.ID
}

func (t threadRecord) Value() threadsNet.Record {
	return t.Record
}

func (t threadRecord) ThreadID() thread.ID {
	return t.threadID
}

func (t threadRecord) LogID() peer.ID {
	return t.logID
}

func (s *service) handleMissingOldDbRecords(threadId string) {
	go func() {
		err := s.handleAllMissingDbRecords(threadId, s.db)
		if err != nil {
			log.Errorf("could not handle missing db records: %v", err)
		}
	}()
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
