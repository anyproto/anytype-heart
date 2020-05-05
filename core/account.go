package core

import (
	"context"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/db"
	"github.com/textileio/go-threads/util"
)

const nodeConnectionTimeout = time.Second * 5

func (a *Anytype) processNewExternalThread(tid thread.ID, ti threadInfo) error {
	log.Infof("got new thread Id: %s, addrs: %+v", ti.ID.String(), ti.Addrs)

	key, err := thread.KeyFromString(ti.Key)
	if err != nil {
		return fmt.Errorf("failed to parse thread keys %s: %s", tid.String(), err.Error())
	}

	threadAdded := false
	atleastOneNodeAdded := false
	hasCafeAddress := false
	var multiAddrs []ma.Multiaddr
	for _, addrS := range ti.Addrs {
		addr, err := ma.NewMultiaddr(addrS)
		if err != nil {
			log.Errorf("processNewExternalThread: failed to parse addr %s: %s", addrS, err.Error())
			continue
		}

		if addr.Equal(a.opts.CafeP2PAddr) {
			hasCafeAddress = true
		}

		multiAddrs = append(multiAddrs, addr)
	}

	if !hasCafeAddress {
		log.Warn("processNewExternalThread cafe addr not found among thread addresses, will add it")
		threadComp, err := ma.NewComponent(thread.Name, tid.String())
		if err != nil {
			return err
		}

		multiAddrs = append(multiAddrs, a.opts.CafeP2PAddr.Encapsulate(threadComp))
	}

addrsLoop:
	for _, addr := range multiAddrs {
		for _, ownAddr := range a.t.Host().Addrs() {
			ipOwn, _ := ownAddr.ValueForProtocol(ma.P_IP4)
			ipTarget, _ := addr.ValueForProtocol(ma.P_IP4)

			portOwn, _ := ownAddr.ValueForProtocol(ma.P_TCP)
			portTarget, _ := addr.ValueForProtocol(ma.P_TCP)

			// do not connect to ourselves
			if ipOwn == ipTarget && portOwn == portTarget {
				continue addrsLoop
			}
		}

		// commented-out because we can have a local cafe node for dev purposes
		/*if v, _ := addr.ValueForProtocol(ma.P_IP4); v == "127.0.0.1" {
			// skip localhost
			continue
		}

		if v, _ := addr.ValueForProtocol(ma.P_IP6); v == "::1" {
			// skip localhost
			continue
		}*/

		threadComp, err := ma.NewComponent(thread.Name, tid.String())
		if err != nil {
			log.Errorf("processNewExternalThread: failed to parse addr %s: %s", addr.String(), err.Error())
			continue
		}

		peerAddr := addr.Decapsulate(threadComp)
		addri, err := peer.AddrInfoFromP2pAddr(peerAddr)
		if err != nil {
			log.Errorf("processNewExternalThread: failed to parse addr %s: %s", addr.String(), err.Error())
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), nodeConnectionTimeout)
		defer cancel()

		if err = a.t.Host().Connect(ctx, *addri); err != nil {
			log.Errorf("processNewExternalThread: failed to connect addr %s: %s", addri, err.Error())
			continue
		}

		if !threadAdded {
			threadAdded = true
			_, err = a.t.AddThread(context.Background(), addr, net.WithThreadKey(key))
			if err != nil {
				return fmt.Errorf("failed to add the remote thread %s: %s", ti.ID.String(), err.Error())
			}
			log.Infof("processNewExternalThread: thread successfully added from %s", addri)
			atleastOneNodeAdded = true
		} else {
			// todo: add addr directly?
			_, err = a.t.AddReplicator(context.Background(), tid, addr)
			if err != nil {
				return fmt.Errorf("failed to add the remote thread %s: %s", ti.ID.String(), err.Error())
			}
			atleastOneNodeAdded = true
		}
	}

	if !atleastOneNodeAdded {
		return fmt.Errorf("failed to add thread from any provided remote address")
	} else {
		err = a.pullThread(context.Background(), tid)
		if err != nil {
			log.Errorf("processNewExternalThread: pull thread failed: %s", err.Error())
		}
	}

	return nil
}

func (a *Anytype) listenExternalNewThreads() error {
	l, err := a.db.Listen()
	if err != nil {
		return err
	}
	go func() {
		defer l.Close()
		for {
			select {
			case <-a.shutdownStartsCh:
				log.Infof("shutting down external changes listener")
				return
			case c := <-l.Channel():
				switch c.Type {
				case db.ActionCreate:
					instanceBytes, err := a.threadsCollection.FindByID(c.ID)
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

					info, _ := a.t.GetThread(context.Background(), tid)
					if info.ID != thread.Undef {
						// our own event
						continue
					}

					go func() {
						err := a.processNewExternalThread(tid, ti)
						if err != nil {
							log.Errorf("processNewExternalThread failed: %s", err.Error())
						}
					}()
				}
			}
		}
	}()

	return nil
}
