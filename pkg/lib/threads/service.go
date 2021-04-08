package threads

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	app2 "github.com/textileio/go-threads/core/app"
	tlcore "github.com/textileio/go-threads/core/logstore"
	"github.com/textileio/go-threads/db/keytransform"
	"google.golang.org/grpc"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	util2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	ma "github.com/multiformats/go-multiaddr"
	db2 "github.com/textileio/go-threads/core/db"
	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/crypto/symmetric"
	"github.com/textileio/go-threads/db"
	"github.com/textileio/go-threads/util"
)

const simultaneousRequests = 20

type service struct {
	Config
	GRPCServerOptions []grpc.ServerOption
	GRPCDialOptions   []grpc.DialOption

	logstore    tlcore.Logstore
	logstoreDS  datastore.DSTxnBatching
	threadsDbDS keytransform.TxnDatastoreExtended

	ctxCancel context.CancelFunc
	ctx       context.Context

	t                          app2.Net
	db                         *db.DB
	threadsCollection          *db.Collection
	device                     wallet.Keypair
	account                    wallet.Keypair
	ipfsNode                   ipfs.Node
	repoRootPath               string
	newHeadProcessor           func(id thread.ID) error
	newThreadChan              chan<- string
	newThreadProcessingLimiter chan struct{}

	replicatorAddr ma.Multiaddr
	sync.Mutex
}

func (s *service) CafePeer() ma.Multiaddr {
	addr, _ := ma.NewMultiaddr(s.CafeP2PAddr)
	return addr
}

type Service interface {
	app.ComponentRunnable
	Logstore() tlcore.Logstore

	ThreadsCollection() (*db.Collection, error)
	Threads() app2.Net
	CafePeer() ma.Multiaddr

	CreateThread(blockType smartblock.SmartBlockType) (thread.Info, error)
	ListThreadIdsByType(blockType smartblock.SmartBlockType) ([]thread.ID, error)

	DeleteThread(id string) error
	InitNewThreadsChan(ch chan<- string) error // can be called only once

	EnsurePredefinedThreads(ctx context.Context, newAccount bool) (DerivedSmartblockIds, error)
}

type ThreadsGetter interface {
	Threads() (thread.IDSlice, error)
}

func (s *service) InitNewThreadsChan(ch chan<- string) error {
	s.Lock()
	defer s.Unlock()
	if s.newThreadChan != nil {
		return fmt.Errorf("already set")
	}

	s.newThreadChan = ch
	return nil
}

func (s *service) getNewThreadChan() chan<- string {
	s.Lock()
	defer s.Unlock()
	return s.newThreadChan
}

func (s *service) ThreadsCollection() (*db.Collection, error) {
	if s.threadsCollection == nil {
		return nil, fmt.Errorf("thread collection not initialized: need to call EnsurePredefinedThreads first")
	}

	return s.threadsCollection, nil
}

func (s *service) Threads() app2.Net {
	return s.t
}

func (s *service) CreateThread(blockType smartblock.SmartBlockType) (thread.Info, error) {
	if s.threadsCollection == nil {
		return thread.Info{}, fmt.Errorf("thread collection not initialized: need to call EnsurePredefinedThreads first")
	}

	// todo: we have a possible trouble here, using thread.AccessControlled uvariant without actually storing the cid with access control
	thrdId, err := ThreadCreateID(thread.AccessControlled, blockType)
	if err != nil {
		return thread.Info{}, err
	}
	followKey, err := symmetric.NewRandom()
	if err != nil {
		return thread.Info{}, err
	}

	readKey, err := symmetric.NewRandom()
	if err != nil {
		return thread.Info{}, err
	}

	thrd, err := s.t.CreateThread(context.TODO(), thrdId, net.WithThreadKey(thread.NewKey(followKey, readKey)), net.WithLogKey(s.device))
	if err != nil {
		return thread.Info{}, err
	}

	metrics.ServedThreads.Inc()
	metrics.ThreadAdded.Inc()

	var replAddrWithThread ma.Multiaddr
	if s.replicatorAddr != nil {
		replAddrWithThread, err = util2.MultiAddressAddThread(s.replicatorAddr, thrdId)
		if err != nil {
			return thread.Info{}, err
		}
		hasReplAddress := util2.MultiAddressHasReplicator(thrd.Addrs, s.replicatorAddr)

		if !hasReplAddress && replAddrWithThread != nil {
			thrd.Addrs = append(thrd.Addrs, replAddrWithThread)
		}
	}

	threadInfo := threadInfo{
		ID:    db2.InstanceID(thrd.ID.String()),
		Key:   thrd.Key.String(),
		Addrs: util2.MultiAddressesToStrings(thrd.Addrs),
	}

	// todo: wait for threadsCollection to push?
	_, err = s.threadsCollection.Create(util.JSONFromInstance(threadInfo))
	if err != nil {
		log.With("thread", thrd.ID.String()).Errorf("failed to create thread at collection: %s: ", err.Error())
	}

	if replAddrWithThread != nil {
		go func() {
			attempt := 0
			start := time.Now()
			// todo: rewrite to job queue in badger
			for {
				attempt++
				metrics.ThreadAddReplicatorAttempts.Inc()
				p, err := s.t.AddReplicator(context.TODO(), thrd.ID, replAddrWithThread)
				if err != nil {
					log.Errorf("failed to add log replicator after %d attempt: %s", attempt, err.Error())
					select {
					case <-time.After(time.Second * 3 * time.Duration(attempt)):
					case <-s.ctx.Done():
						return
					}
					continue
				}

				metrics.ThreadAddReplicatorDuration.Observe(time.Since(start).Seconds())
				log.With("thread", thrd.ID.String()).Infof("added log replicator after %d attempt: %s", attempt, p.String())
				return
			}
		}()
	}

	return thrd, nil
}

func (s *service) DeleteThread(id string) error {
	if s.threadsCollection == nil {
		return fmt.Errorf("thread collection not initialized: need to call EnsurePredefinedThreads first")
	}

	tid, err := thread.Decode(id)
	if err != nil {
		return fmt.Errorf("incorrect block id: %w", err)
	}

	err = s.t.DeleteThread(context.Background(), tid)
	if err != nil {
		return err
	}

	err = s.threadsCollection.Delete(db2.InstanceID(id))
	if err != nil {
		// todo: here we can get an error if we didn't yet added thead keys into DB
		log.With("thread", id).Error("DeleteThread failed to remove thread from collection: %s", err.Error())
	}
	return nil
}

func (s *service) ListThreadIdsByType(blockType smartblock.SmartBlockType) ([]thread.ID, error) {
	threads, err := s.logstore.Threads()
	if err != nil {
		return nil, err
	}

	var filtered []thread.ID
	for _, thrdId := range threads {
		t, err := smartblock.SmartBlockTypeFromThreadID(thrdId)
		if err != nil {
			log.Errorf("smartblock has incorrect id(%s), failed to decode type: %v", thrdId.String(), err)
			continue
		}

		if t == blockType {
			filtered = append(filtered, thrdId)
		}
	}

	return filtered, nil
}
