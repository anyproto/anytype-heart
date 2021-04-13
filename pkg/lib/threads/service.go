package threads

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs/helpers"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util/nocloserds"
	walletUtil "github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/textileio/go-threads/logstore/lstoreds"
	threadsNet "github.com/textileio/go-threads/net"
	"sync"
	"time"

	ma "github.com/multiformats/go-multiaddr"
	threadsApp "github.com/textileio/go-threads/core/app"
	"github.com/textileio/go-threads/core/db"
	tlcore "github.com/textileio/go-threads/core/logstore"
	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	"github.com/textileio/go-threads/crypto/symmetric"
	threadsDb "github.com/textileio/go-threads/db"
	"github.com/textileio/go-threads/db/keytransform"
	threadsUtil "github.com/textileio/go-threads/util"
	"google.golang.org/grpc"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util"
)

const simultaneousRequests = 20

const CName = "threads"

var log = logging.Logger("anytype-threads")

var (
	permanentConnectionRetryDelay = time.Second * 5
)

type service struct {
	Config
	GRPCServerOptions []grpc.ServerOption
	GRPCDialOptions   []grpc.DialOption

	logstore    tlcore.Logstore
	ds          datastore.Datastore
	logstoreDS  datastore.DSTxnBatching
	threadsDbDS keytransform.TxnDatastoreExtended
	stopped     bool

	ctxCancel                  context.CancelFunc
	ctx                        context.Context
	presubscribedChangesChan   <-chan net.ThreadRecord
	t                          threadsApp.Net
	db                         *threadsDb.DB
	threadsCollection          *threadsDb.Collection
	device                     walletUtil.Keypair
	account                    walletUtil.Keypair
	ipfsNode                   ipfs.Node
	repoRootPath               string
	newThreadChan              chan<- string
	newThreadProcessingLimiter chan struct{}

	replicatorAddr ma.Multiaddr
	sync.Mutex
}

func New() Service {
	ctx, cancel := context.WithCancel(context.Background())
	limiter := make(chan struct{}, simultaneousRequests)
	for i := 0; i < cap(limiter); i++ {
		limiter <- struct{}{}
	}

	return &service{
		newThreadProcessingLimiter: limiter,
		ctx:                        ctx,
		ctxCancel:                  cancel,
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.Config = a.Component("config").(ThreadsConfigGetter).ThreadsConfig()
	s.ds = a.MustComponent(datastore.CName).(datastore.Datastore)
	wl := a.MustComponent(wallet.CName).(wallet.Wallet)
	s.ipfsNode = a.MustComponent(ipfs.CName).(ipfs.Node)

	s.device, err = wl.GetDevicePrivkey()
	if err != nil {
		return fmt.Errorf("device key is required")
	}
	// it is ok to miss the account key in case of backup node
	s.account, _ = wl.GetAccountPrivkey()

	var (
		unaryServerInterceptor grpc.UnaryServerInterceptor
		unaryClientInterceptor grpc.UnaryClientInterceptor
	)

	if metrics.Enabled {
		unaryServerInterceptor = grpc_prometheus.UnaryServerInterceptor
		unaryClientInterceptor = grpc_prometheus.UnaryClientInterceptor
		grpc_prometheus.EnableHandlingTimeHistogram()
		grpc_prometheus.EnableClientHandlingTimeHistogram()
	}
	s.GRPCServerOptions = []grpc.ServerOption{
		grpc.MaxRecvMsgSize(5 << 20), // 5Mb max message size
		grpc.UnaryInterceptor(unaryServerInterceptor),
	}
	s.GRPCDialOptions = []grpc.DialOption{
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(5<<20),
			grpc.MaxCallSendMsgSize(5<<20),
		),
		grpc.WithUnaryInterceptor(unaryClientInterceptor),
	}
	return nil
}

func (s *service) Run() (err error) {
	s.logstoreDS, err = s.ds.LogstoreDS()
	if err != nil {
		return err
	}

	s.threadsDbDS, err = s.ds.ThreadsDbDS()
	if err != nil {
		return err
	}

	s.logstore, err = lstoreds.NewLogstore(s.ctx, nocloserds.NewTxnBatch(s.logstoreDS), lstoreds.DefaultOpts())
	if err != nil {
		return err
	}

	// persistent sync tracking only
	var syncBook tlcore.SyncBook
	if s.SyncTracking {
		syncBook = s.logstore
	}

	s.t, err = threadsNet.NewNetwork(s.ctx, s.ipfsNode.GetHost(), s.ipfsNode.BlockStore(), s.ipfsNode, s.logstore, threadsNet.Config{
		Debug:        s.Debug,
		PubSub:       s.PubSub,
		SyncTracking: s.SyncTracking,
		SyncBook:     syncBook,
	}, s.GRPCServerOptions, s.GRPCDialOptions)
	if err != nil {
		return err
	}
	s.presubscribedChangesChan, err = s.t.Subscribe(s.ctx)
	if err != nil {
		return err
	}
	if s.CafeP2PAddr != "" {
		addr, err := ma.NewMultiaddr(s.CafeP2PAddr)
		if err != nil {
			return err
		}
		s.replicatorAddr = addr
		// protect cafe connections from pruning
		if p, err := addr.ValueForProtocol(ma.P_P2P); err == nil {
			if pid, err := peer.Decode(p); err == nil {
				s.ipfsNode.GetHost().ConnManager().Protect(pid, "cafe-sync")
			} else {
				log.Errorf("decoding peerID from cafe address failed: %v", err)
			}
		}

		if s.CafePermanentConnection {
			// todo: do we need to wait bootstrap?
			err = helpers.PermanentConnection(s.ctx, addr, s.ipfsNode.GetHost(), permanentConnectionRetryDelay)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *service) Close() (err error) {
	s.Lock()
	defer s.Unlock()
	log.Errorf("threadService.Close()")
	if s.stopped {
		return nil
	}
	s.stopped = true

	// close context in order to protect channel from close
	if s.ctxCancel != nil {
		s.ctxCancel()
	}

	if s.db != nil {
		err := s.db.Close()
		if err != nil {
			return err
		}
	}
	if s.t != nil {
		err := s.t.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Logstore() tlcore.Logstore {
	return s.logstore
}

func (s *service) PresubscribedNewRecords() (<-chan net.ThreadRecord, error) {
	if s.presubscribedChangesChan == nil {
		return nil, fmt.Errorf("presubscribed channel is nil")
	}

	return s.presubscribedChangesChan, nil
}

func (s *service) CafePeer() ma.Multiaddr {
	addr, _ := ma.NewMultiaddr(s.CafeP2PAddr)
	return addr
}

type Service interface {
	app.ComponentRunnable
	Logstore() tlcore.Logstore

	ThreadsCollection() (*threadsDb.Collection, error)
	Threads() threadsApp.Net
	CafePeer() ma.Multiaddr

	CreateThread(blockType smartblock.SmartBlockType) (thread.Info, error)
	ListThreadIdsByType(blockType smartblock.SmartBlockType) ([]thread.ID, error)

	DeleteThread(id string) error
	InitNewThreadsChan(ch chan<- string) error // can be called only once

	PresubscribedNewRecords() (<-chan net.ThreadRecord, error)
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

func (s *service) ThreadsCollection() (*threadsDb.Collection, error) {
	if s.threadsCollection == nil {
		return nil, fmt.Errorf("thread collection not initialized: need to call EnsurePredefinedThreads first")
	}

	return s.threadsCollection, nil
}

func (s *service) Threads() threadsApp.Net {
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
		replAddrWithThread, err = util.MultiAddressAddThread(s.replicatorAddr, thrdId)
		if err != nil {
			return thread.Info{}, err
		}
		hasReplAddress := util.MultiAddressHasReplicator(thrd.Addrs, s.replicatorAddr)

		if !hasReplAddress && replAddrWithThread != nil {
			thrd.Addrs = append(thrd.Addrs, replAddrWithThread)
		}
	}

	threadInfo := threadInfo{
		ID:    db.InstanceID(thrd.ID.String()),
		Key:   thrd.Key.String(),
		Addrs: util.MultiAddressesToStrings(thrd.Addrs),
	}

	// todo: wait for threadsCollection to push?
	_, err = s.threadsCollection.Create(threadsUtil.JSONFromInstance(threadInfo))
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

	err = s.threadsCollection.Delete(db.InstanceID(id))
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
