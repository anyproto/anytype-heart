package threads

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-threads/core/thread"
)

var ErrFailedToPullThread = fmt.Errorf("failed to pull thread")
var ErrFailedToProcessNewHead = fmt.Errorf("failed to process new page head")

// PullThread pulls the thread and run newHeadProcessor in case heads have been changed
func (s *service) PullThread(ctx context.Context, id thread.ID) (err error) {
	headsChanged, err := s.pullThread(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrFailedToPullThread, err.Error())
	}

	if !headsChanged {
		return nil
	}

	if s.newHeadProcessor != nil {
		err = s.newHeadProcessor(id)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrFailedToProcessNewHead, err.Error())
		}
	}

	return nil
}

func (s *service) pullThread(ctx context.Context, id thread.ID) (headsChanged bool, err error) {
	thrd, err := s.t.GetThread(context.Background(), id)
	if err != nil {
		return false, err
	}

	var headPerLog = make(map[peer.ID]cid.Cid, len(thrd.Logs))
	for _, log := range thrd.Logs {
		headPerLog[log.ID] = log.Head
	}

	err = s.t.PullThread(ctx, id)
	if err != nil {
		return false, err
	}

	thrd, err = s.t.GetThread(context.Background(), id)
	if err != nil {
		return false, err
	}

	for _, log := range thrd.Logs {
		if v, exists := headPerLog[log.ID]; !exists && log.Head.Defined() {
			headsChanged = true
			break
		} else {
			if !log.Head.Equals(v) {
				headsChanged = true
				break
			}
		}
	}

	return
}

func (s *service) addMissingReplicators() error {
	threadsIds, err := s.threadsGetter.Threads()
	if err != nil {
		return fmt.Errorf("failed to list threads: %s", err.Error())
	}

	if s.replicatorAddr == nil {
		return nil
	}

	for _, threadId := range threadsIds {
		thrdLogs, err := s.t.Logstore().GetManagedLogs(threadId)
		if err != nil {
			log.Errorf("failed to get thread %s: %s", threadId.String(), err.Error())
			continue
		}

		for _, thrdLog := range thrdLogs {
			if !util.MultiAddressHasReplicator(thrdLog.Addrs, s.replicatorAddr) {
				ctx, cancel := context.WithTimeout(s.ctx, time.Second*30)
				thrd, err := s.t.GetThread(ctx, threadId)
				cancel()
				if err != nil {
					log.Errorf("addMissingReplicators failed to get thread %s: %s", threadId, err.Error())
					continue
				}

				err = s.addReplicatorWithAttempts(s.ctx, thrd, s.replicatorAddr, 0)
				if err != nil {
					log.Errorf("failed to add missing replicator for %s: %s", thrd.ID, err.Error())
				} else {
					log.Warnf("added missing replicator for %s", thrd.ID)
					// we can break here, because thread.addReplicator has successfully added replicator for each managed log
					break
				}
			}
		}

	}
	return nil
}

// addReplicatorWithAttempts try to add the cafe replicatorAddr continuously with maxAttempts
// maxAttempts <= 0 will make it repeat indefinitely until neither success or ctx.Done()
func (s *service) addReplicatorWithAttempts(ctx context.Context, thrd thread.Info, replicatorAddr ma.Multiaddr, maxAttempts int) (err error) {
	attempt := 0
	if replicatorAddr == nil {
		return fmt.Errorf("replicatorAddr is nil")
	}

	pId, err := s.device.PeerId()
	if err != nil {
		return err
	}

	ownLog := util.GetLog(thrd.Logs, pId)
	if util.MultiAddressHasReplicator(ownLog.Addrs, replicatorAddr) {
		return nil
	}

	log.With("thread", thrd.ID.String()).Debugf("no cafe addr found for own log")
	start := time.Now()
	for {
		metrics.ThreadAddReplicatorAttempts.Inc()
		_, err = s.t.AddReplicator(ctx, thrd.ID, replicatorAddr)
		if err == nil {
			metrics.ThreadAddReplicatorDuration.Observe(time.Since(start).Seconds())
			return
		}

		attempt++
		log.Errorf("addReplicatorWithAttempts failed for %s after %.2fs %d/%d attempt: %s", thrd.ID.String(), time.Since(start).Seconds(), attempt, maxAttempts, err.Error())

		if maxAttempts > 0 && attempt >= maxAttempts {
			return ErrAddReplicatorsAttemptsExceeded
		}

		select {
		case <-time.After(time.Second * time.Duration(3*attempt)):
			continue
		case <-ctx.Done():
			err = ctx.Err()
			return
		}
	}
}
