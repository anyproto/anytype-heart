package threads

import (
	"context"
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	db "github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/logstore"
	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
)

const nodeConnectionTimeout = time.Second * 15
const ThreadInfoCollectionName = "threads"
const MetaCollectionName = "meta"
const CreatorCollectionName = "creator"
const HighlightedCollectionName = "highlighted"

type ThreadDBInfo struct {
	ID    db.InstanceID `json:"_id"`
	Key   string
	Addrs []string
}

type ThreadInfo struct {
	ID    string
	Key   string
	Addrs []string
}

type WorkspaceMeta interface {
	WorkspaceName() string
	Account() string
}

type MetaInfo struct {
	ID            db.InstanceID `json:"_id"`
	Name          string
	AccountPubKey string
}

type CreatorInfo struct {
	ID            string
	AccountPubKey string
	WorkspaceSig  []byte
	Addrs         []string
	ProfileId     string
}

type CollectionUpdateInfo struct {
	ID    db.InstanceID `json:"_id"`
	Value interface{}
}

func (m *MetaInfo) WorkspaceName() string {
	return m.Name
}

func (m *MetaInfo) Account() string {
	return m.AccountPubKey
}

func (s *service) processNewExternalThread(tid thread.ID, ti ThreadInfo, pullAsync bool) error {
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
		if !util.MultiAddressHasReplicator(multiAddrs, s.replicatorAddr) {
			replAddrWithThread, err = util.MultiAddressAddThread(s.replicatorAddr, tid)
			if err != nil {
				return err
			}

			log.Warn("processNewExternalThread: cafe addr not found among thread addresses, will add it")
			multiAddrs = append(multiAddrs, replAddrWithThread)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	if localThreadInfo, err = s.t.GetThread(ctx, tid); err == nil {
		success = true
		if hasNonEmptyLogs(localThreadInfo.Logs) {
			log.Debugf("processNewExternalThread: thread already exists locally and has non-empty logs")
			return nil
		} else {
			log.Warnf("processNewExternalThread: thread already exists locally but all logs are empty")
		}
	} else {
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

			peerAddr, err := util.MultiAddressTrimThread(addr)
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

			addr, err = util.MultiAddressAddThread(addr, tid)
			localThreadInfo, err = s.t.AddThread(s.ctx, addr, net.WithThreadKey(key), net.WithoutLog())
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

	if s.replicatorAddr != nil {
		s.threadQueue.AddReplicator(tid)
	}

	// TODO: should we add timeout here?
	pullFunc := func() error {
		_, err = s.pullThread(s.ctx, tid)
		if err != nil {
			log.Errorf("processNewExternalThread: pull thread failed: %s", err.Error())
			return fmt.Errorf("failed to pull thread: %w", err)
		}
		return nil
	}
	if pullAsync {
		go pullFunc()
	} else {
		return pullFunc()
	}

	return nil
}

func hasNonEmptyLogs(logs []thread.LogInfo) bool {
	for _, l := range logs {
		if l.Head.ID.Defined() {
			return true
		}
	}

	return false
}
