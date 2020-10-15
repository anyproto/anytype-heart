package threads

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	util2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	db2 "github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/logstore"
	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/db"
	"github.com/textileio/go-threads/util"
)

const nodeConnectionTimeout = time.Second * 15
const ThreadInfoCollectionName = "threads"

var (
	threadInfoCollection = db.CollectionConfig{
		Name:   ThreadInfoCollectionName,
		Schema: util.SchemaFromInstance(threadInfo{}, false),
	}
)

type threadInfo struct {
	ID    db2.InstanceID `json:"_id"`
	Key   string
	Addrs []string
}

func (s *service) threadsDbInit() error {
	if s.db != nil {
		return nil
	}

	accountID, err := s.derivedThreadIdByIndex(threadDerivedIndexAccount)
	if err != nil {
		return err
	}

	d, err := db.NewDB(context.Background(), s.t, accountID, db.WithNewRepoPath(filepath.Join(s.repoRootPath, "collections")), db.WithNewCollections())
	if err != nil {
		return err
	}

	s.db = d

	s.threadsCollection = s.db.GetCollection(ThreadInfoCollectionName)
	err = s.threadsDbListen()
	if err != nil {
		return fmt.Errorf("failed to listen external new threads: %w", err)
	}

	if s.threadsCollection == nil {
		s.threadsCollection, err = s.db.NewCollection(threadInfoCollection)
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *service) threadsDbListen() error {
	log.Infof("threadsDbListen")
	l, err := s.db.Listen()
	if err != nil {
		return err
	}

	go func() {
		defer func() {
			l.Close()
			if s.newThreadChan != nil {
				close(s.newThreadChan)
			}
		}()

		for {
			select {
			case <-s.ctx.Done():
				return
			case c := <-l.Channel():
				switch c.Type {
				case db.ActionCreate:
					instanceBytes, err := s.threadsCollection.FindByID(c.ID)
					if err != nil {
						log.Errorf("failed to find thread info for id %s: %w", c.ID.String(), err)
						continue
					}

					ti := threadInfo{}
					util.InstanceFromJSON(instanceBytes, &ti)
					tid, err := thread.Decode(ti.ID.String())
					if err != nil {
						log.Errorf("failed to parse thread id %s: %s", ti.ID, err.Error())
						continue
					}

					info, _ := s.t.GetThread(context.Background(), tid)
					if info.ID != thread.Undef {
						// our own event
						continue
					}

					go func() {
						s.processNewExternalThreadUntilSuccess(tid, ti)
						if s.newThreadChan != nil {
							select {
							case <-s.ctx.Done():
							case s.newThreadChan <- tid.String():
							}
						}
					}()
				}
			}
		}
	}()

	return nil
}

// processNewExternalThreadUntilSuccess tries to add the new thread from remote peer until success
// supposed to be run in goroutine
func (s *service) processNewExternalThreadUntilSuccess(tid thread.ID, ti threadInfo) {
	log := log.With("thread", tid.String())
	log.With("threadAddrs", ti.Addrs).Info("got new thread")

	var attempt int
	for {
		attempt++
		err := s.processNewExternalThread(tid, ti)
		if err != nil {
			log.Errorf("processNewExternalThread failed after %d attempt: %s", attempt, err.Error())
		} else {
			log.Debugf("processNewExternalThread succeed after %d attempt", attempt)
			return
		}
		select {
		case <-s.ctx.Done():
			return
		case <-time.After(time.Duration(5*attempt) * time.Second):
			continue
		}
	}
}

func (s *service) processNewExternalThread(tid thread.ID, ti threadInfo) error {
	log := log.With("thread", tid.String())
	key, err := thread.KeyFromString(ti.Key)
	if err != nil {
		return fmt.Errorf("failed to parse thread keys %s: %s", tid.String(), err.Error())
	}

	success := false
	var multiAddrs []ma.Multiaddr
	for _, addr := range ti.Addrs {
		addr, err := ma.NewMultiaddr(addr)
		if err != nil {
			log.Errorf("processNewExternalThread: failed to decode addr %s: %s", addr, err.Error())
			continue
		}
		multiAddrs = append(multiAddrs, addr)
	}

	var localThreadInfo thread.Info
	var replAddrWithThread ma.Multiaddr
	if s.replicatorAddr != nil {
		replAddrWithThread, err = util2.MultiAddressAddThread(s.replicatorAddr, tid)
		if err != nil {
			return err
		}

		if !util2.MultiAddressHasReplicator(multiAddrs, s.replicatorAddr) {
			log.Warn("processNewExternalThread: cafe addr not found among thread addresses, will add it")
			multiAddrs = append(multiAddrs, replAddrWithThread)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if localThreadInfo, err = s.t.GetThread(ctx, tid); err == nil {
		log.Warnf("processNewExternalThread: thread already exists locally")
		if hasNonEmptyLogs(localThreadInfo.Logs) {
			return nil
		}
	} else {
		log.Errorf("processNewExternalThread: thread doesn't exist locally: %s", err.Error())
	addrsLoop:
		for _, addr := range multiAddrs {
			logWithAddr := log.With("addr", addr.String())
			for _, ownAddr := range s.t.Host().Addrs() {
				ipOwn, _ := ownAddr.ValueForProtocol(ma.P_IP4)
				ipTarget, _ := addr.ValueForProtocol(ma.P_IP4)

				portOwn, _ := ownAddr.ValueForProtocol(ma.P_TCP)
				portTarget, _ := addr.ValueForProtocol(ma.P_TCP)

				// do not connect to ourselves
				if ipOwn == ipTarget && portOwn == portTarget {
					continue addrsLoop
				}
			}

			peerAddr, err := util2.MultiAddressTrimThread(addr)
			if err != nil {
				logWithAddr.Errorf("processNewExternalThread: failed to parse addr: %s", err.Error())
				continue
			}

			peerAddrInfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
			if err != nil {
				logWithAddr.Errorf("processNewExternalThread: failed to parse addr: %s", err.Error())
				continue
			}

			ctx, cancel := context.WithTimeout(context.Background(), nodeConnectionTimeout)
			defer cancel()

			if err = s.t.Host().Connect(ctx, *peerAddrInfo); err != nil {
				logWithAddr.With("threadAddr", tid.String()).Errorf("processNewExternalThread: failed to connect addr: %s", err.Error())
				continue
			}

			addr, err = util2.MultiAddressAddThread(addr, tid)
			localThreadInfo, err = s.t.AddThread(s.ctx, addr, net.WithThreadKey(key), net.WithLogKey(s.device))
			if err != nil {
				if err == logstore.ErrLogExists || err == logstore.ErrThreadExists {
					err2 := err
					localThreadInfo, err = s.t.GetThread(ctx, tid)
					if err != nil {
						logWithAddr.Errorf("processNewExternalThread: failed to add(%s) and get thread: %s", err2.Error(), err.Error())
					}
					success = true
					break
				}
				logWithAddr.Errorf("processNewExternalThread: failed to add: %s", err.Error())
				continue
			}

			logWithAddr.Infof("processNewExternalThread: thread successfully added %s", peerAddrInfo.String())

			success = true
			break
		}
	}

	if !success {
		return fmt.Errorf("failed to add thread from any provided remote address")
	}

	if s.replicatorAddr != nil && !util2.MultiAddressHasReplicator(localThreadInfo.Addrs, s.replicatorAddr) {
		_, err = s.t.AddReplicator(s.ctx, tid, replAddrWithThread)
		if err != nil {
			log.Errorf("processNewExternalThread failed to add the replicator: %s", err.Error())
		}
	}

	_, err = s.pullThread(s.ctx, tid)
	if err != nil {
		log.Errorf("processNewExternalThread: pull thread failed: %s", err.Error())
		return fmt.Errorf("failed to pull thread: %w", err)
	}

	err = s.newHeadProcessor(tid)
	if err != nil {
		log.Errorf("processNewExternalThread newHeadProcessor failed: %s", err.Error())
	}

	return nil
}

func hasNonEmptyLogs(logs []thread.LogInfo) bool {
	for _, l := range logs {
		if l.Head.Defined() {
			return true
		}
	}

	return false
}
