package threads

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	util2 "github.com/anytypeio/go-anytype-library/util"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	db2 "github.com/textileio/go-threads/core/db"
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

	d, err := db.NewDB(context.Background(), s.t, accountID, db.WithNewRepoPath(filepath.Join(s.repoRootPath, "collections")))
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
	l, err := s.db.Listen()
	if err != nil {
		return err
	}

	go func() {
		defer l.Close()
		for {
			select {
			case <-s.closeCh:
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
	log.Infof("got new thread %s, addrs: %+v", ti.ID.String(), ti.Addrs)

	var attempt int
	for {
		attempt++
		err := s.processNewExternalThread(tid, ti)
		if err != nil {
			log.Errorf("processNewExternalThread %s failed after %d attempt: %s", tid.String(), err.Error())
		} else {
			log.Debugf("processNewExternalThread %s succeed after %d attempt", tid.String())
			return
		}
		select {
		case <-s.closeCh:
			return
		case <-time.After(time.Duration(5*attempt) * time.Second):
			continue
		}
	}
}

func (s *service) processNewExternalThread(tid thread.ID, ti threadInfo) error {
	key, err := thread.KeyFromString(ti.Key)
	if err != nil {
		return fmt.Errorf("failed to parse thread keys %s: %s", tid.String(), err.Error())
	}

	threadAdded := false
	var multiAddrs []ma.Multiaddr
	for _, addr := range ti.Addrs {
		addr, err := ma.NewMultiaddr(addr)
		if err != nil {
			log.Errorf("processNewExternalThread: failed to decode addr %s: %s", addr, err.Error())
			continue
		}
		multiAddrs = append(multiAddrs, addr)
	}

	var replAddrWithThread ma.Multiaddr
	if s.replicatorAddr != nil {
		replAddrWithThread, err = util2.MultiAddressAddThread(s.replicatorAddr, tid)
		if err != nil {
			return err
		}

		if !util2.MultiAddressHasReplicator(multiAddrs, s.replicatorAddr) {
			log.Warn("processNewExternalThread %s: cafe addr not found among thread addresses, will add it", ti.ID.String())
			replAddr, err := util2.MultiAddressAddThread(s.replicatorAddr, tid)
			if err != nil {
				return err
			}

			multiAddrs = append(multiAddrs, replAddr)
		}
	}

addrsLoop:
	for _, addr := range multiAddrs {
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
			log.Errorf("processNewExternalThread %s: failed to parse addr %s: %s", ti.ID.String(), addr.String(), err.Error())
			continue
		}

		peerAddrInfo, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			log.Errorf("processNewExternalThread %s: failed to parse addr %s: %s", ti.ID.String(), addr.String(), err.Error())
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), nodeConnectionTimeout)
		defer cancel()

		if err = s.t.Host().Connect(ctx, *peerAddrInfo); err != nil {
			log.Errorf("processNewExternalThread %s: failed to connect addr %s: %s", ti.ID.String(), addr.String(), err.Error())
			continue
		}

		addr, err = util2.MultiAddressAddThread(addr, tid)
		_, err = s.t.AddThread(context.Background(), addr, net.WithThreadKey(key), net.WithLogKey(s.device))
		if err != nil {
			log.Errorf("processNewExternalThread %s: failed to add from %s: %s", ti.ID.String(), addr.String(), err.Error())
			continue
		}

		log.Infof("processNewExternalThread %s: thread successfully added from %s", ti.ID.String(), peerAddrInfo)
		_, err = s.t.AddReplicator(context.Background(), tid, replAddrWithThread)
		if err != nil {
			log.Errorf("processNewExternalThread failed to add the replicator for %s: %s", ti.ID.String(), err.Error())
		}

		threadAdded = true
		break
	}

	if !threadAdded {
		return fmt.Errorf("failed to add thread from any provided remote address")
	} else {
		_, err = s.pullThread(context.Background(), tid)
		if err != nil {
			log.Errorf("processNewExternalThread: pull thread %s failed: %s", tid.String(), err.Error())
		}
	}

	err = s.newHeadProcessor(tid)
	if err != nil {
		log.Errorf("processNewExternalThread newHeadProcessor %s failed: %s", tid.String(), err.Error())
	}

	return nil
}
