package core

import (
	"github.com/golang/protobuf/ptypes"
	tpb "github.com/textileio/go-textile/pb"
)

func (t *Anytype) syncAccount(waitResult bool) error {
	t.lock.Lock()
	defer t.lock.Unlock()

	if t.cancelSync != nil {
		t.cancelSync.Close()
	}

	query := &tpb.ThreadSnapshotQuery{
		Address: t.Textile.Address(),
	}

	options := &tpb.QueryOptions{
		Wait: 15,
	}

	resCh, errCh, cancel, err := t.textile().SearchThreadSnapshots(query, options)
	if err != nil {
		return err
	}

	t.cancelSync = cancel

	var waitChan = make(chan struct{})
	go func() {
		for {
			select {
			case res, ok := <-resCh:
				if !ok {
					if waitResult {
						waitChan <- struct{}{}
					}
					return
				}

				err := t.applySnapshot(res)
				if err != nil {
					log.Errorf("error applying snap %s: %s", res.Id, err)
				}

			case err := <-errCh:
				log.Errorf("error during snapshot sync: %s", err)
			}
		}
	}()

	if waitResult {
		<-waitChan
	}

	return nil
}

// applySnapshot unmarshals and adds an unencrypted thread snapshot from a search result
func (t *Anytype) applySnapshot(result *tpb.QueryResult) error {
	snap := new(tpb.Thread)
	if err := ptypes.UnmarshalAny(result.Value, snap); err != nil {
		return err
	}

	log.Debugf("apply thread snapshot: %s", result.Id)
	return t.textile().AddOrUpdateThread(snap, true)
}

type Closer interface {
	Close()
}
