package threads

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs/helpers"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util/nocloserds"
	walletUtil "github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-tcp-transport"
	"github.com/textileio/go-threads/logstore/lstoreds"
	threadsNet "github.com/textileio/go-threads/net"
	threadsQueue "github.com/textileio/go-threads/net/queue"
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
	threadsMetrics "github.com/textileio/go-threads/metrics"
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

const maxReceiveMessageSize int = 100 * 1024 * 1024

type service struct {
	Config
	GRPCServerOptions []grpc.ServerOption
	GRPCDialOptions   []grpc.DialOption

	// the number of simultaneous requests when processing threads or adding replicator
	simultaneousRequests int

	logstore    tlcore.Logstore
	ds          datastore.Datastore
	logstoreDS  datastore.DSTxnBatching
	threadsDbDS keytransform.TxnDatastoreExtended
	stopped     bool

	ctxCancel                      context.CancelFunc
	ctx                            context.Context
	presubscribedChangesChan       <-chan net.ThreadRecord
	t                              threadsApp.Net
	db                             *threadsDb.DB
	threadsCollection              *threadsDb.Collection
	currentWorkspaceId             thread.ID
	device                         walletUtil.Keypair
	account                        walletUtil.Keypair
	ipfsNode                       ipfs.Node
	repoRootPath                   string
	newThreadChan                  chan<- string
	newThreadProcessingLimiter     chan struct{}
	newReplicatorProcessingLimiter chan struct{}
	process                        process.Service
	threadProcessors               map[thread.ID]ThreadProcessor
	processorMutex                 sync.RWMutex

	fetcher               CafeConfigFetcher
	workspaceThreadGetter CurrentWorkspaceThreadGetter
	threadCreateQueue     ThreadCreateQueue

	replicatorAddr ma.Multiaddr
	sync.Mutex
}

func New() Service {
	/* adjust ThreadsDB parameters */

	// thread pulling cycle
	threadsNet.PullStartAfter = 5 * time.Second
	threadsNet.InitialPullInterval = 20 * time.Second
	threadsNet.PullInterval = 3 * time.Minute

	// communication timeouts
	threadsNet.DialTimeout = 20 * time.Second          // we can set safely set a long dial timeout because unavailable peer are cached for some time and local network timeouts are overridden with 5s
	tcp.DefaultConnectTimeout = threadsNet.DialTimeout // override default tcp dial timeout because it has a priority over the passing context's deadline
	threadsNet.PushTimeout = 30 * time.Second
	threadsNet.PullTimeout = 2 * time.Minute

	// event bus input buffer
	threadsNet.EventBusCapacity = 3

	// exchange edges
	threadsNet.MaxThreadsExchanged = 10
	threadsNet.ExchangeCompressionTimeout = 20 * time.Second
	threadsNet.QueuePollInterval = 1 * time.Second

	// thread packer queue
	threadsQueue.InBufSize = 5
	threadsQueue.OutBufSize = 2
	ctx, cancel := context.WithCancel(context.Background())

	return &service{
		ctx:                  ctx,
		ctxCancel:            cancel,
		simultaneousRequests: simultaneousRequests,
		threadProcessors:     make(map[thread.ID]ThreadProcessor),
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.Config = a.Component("config").(ThreadsConfigGetter).ThreadsConfig()
	s.ds = a.MustComponent(datastore.CName).(datastore.Datastore)
	s.fetcher = a.MustComponent("configfetcher").(CafeConfigFetcher)
	s.workspaceThreadGetter = a.MustComponent("objectstore").(CurrentWorkspaceThreadGetter)
	s.threadCreateQueue = a.MustComponent("objectstore").(ThreadCreateQueue)
	s.process = a.MustComponent(process.CName).(process.Service)
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
		grpc.UnaryInterceptor(unaryServerInterceptor),
	}
	s.GRPCDialOptions = []grpc.DialOption{
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxReceiveMessageSize),
		),
		grpc.WithUnaryInterceptor(unaryClientInterceptor),
	}
	return nil
}

func (s *service) Run() (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	cafeCfg := s.fetcher.GetConfig(ctx)
	cancel()
	if cafeCfg.SimultaneousRequests != 0 {
		s.simultaneousRequests = int(cafeCfg.SimultaneousRequests)
	} else {
		s.simultaneousRequests = simultaneousRequests
	}

	s.newThreadProcessingLimiter = make(chan struct{}, s.simultaneousRequests)
	for i := 0; i < cap(s.newThreadProcessingLimiter); i++ {
		s.newThreadProcessingLimiter <- struct{}{}
	}
	s.newReplicatorProcessingLimiter = make(chan struct{}, s.simultaneousRequests)
	for i := 0; i < cap(s.newReplicatorProcessingLimiter); i++ {
		s.newReplicatorProcessingLimiter <- struct{}{}
	}

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

	ctx = context.WithValue(s.ctx, threadsMetrics.ContextKey{}, metrics.NewThreadsMetrics())

	s.t, err = threadsNet.NewNetwork(ctx, s.ipfsNode.GetHost(), s.ipfsNode.BlockStore(), s.ipfsNode, s.logstore, threadsNet.Config{
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

	CreateWorkspace() (thread.Info, error)
	SelectWorkspace(ctx context.Context, ids DerivedSmartblockIds, workspaceId thread.ID) (DerivedSmartblockIds, error)
	CreateThread(blockType smartblock.SmartBlockType) (thread.Info, error)
	DeleteThread(id string) error
	InitNewThreadsChan(ch chan<- string) error // can be called only once

	GetThreadInfo(id thread.ID) (thread.Info, error)
	AddThread(threadId string, key string, addrs []string) error

	PresubscribedNewRecords() (<-chan net.ThreadRecord, error)
	EnsurePredefinedThreads(ctx context.Context, newAccount bool) (DerivedSmartblockIds, *DerivedSmartblockIds, error)
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

func (s *service) closeThreadChan() {
	s.Lock()
	defer s.Unlock()
	if s.newThreadChan != nil {
		close(s.newThreadChan)
	}
	s.newThreadChan = nil
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

func (s *service) CreateWorkspace() (thread.Info, error) {
	// create new workspace thread
	workspaceThread, err := s.CreateThread(smartblock.SmartBlockTypeWorkspace)
	if err != nil {
		return thread.Info{}, fmt.Errorf("failed to create new workspace thread: %w", err)
	}

	_, err = s.startWorkspaceThreadProcessor(workspaceThread.ID.String())
	if err != nil {
		return thread.Info{}, fmt.Errorf("could not start thread processor: %w", err)
	}

	workspaceReadKeyBytes := workspaceThread.Key.Read().Bytes()

	// creating home thread
	homeId, err := threadDeriveId(threadDerivedIndexHome, workspaceReadKeyBytes)
	if err != nil {
		return thread.Info{}, err
	}
	homeSk, homeRk, err := threadDeriveKeys(threadDerivedIndexHome, workspaceReadKeyBytes)
	if err != nil {
		return thread.Info{}, err
	}
	_, err = s.threadCreate(homeId, thread.NewKey(homeSk, homeRk))
	if err != nil {
		return thread.Info{}, fmt.Errorf("could not create home thread: %w", err)
	}

	// creating archive thread
	archiveId, err := threadDeriveId(threadDerivedIndexArchive, workspaceReadKeyBytes)
	if err != nil {
		return thread.Info{}, err
	}
	archiveSk, archiveRk, err := threadDeriveKeys(threadDerivedIndexArchive, workspaceReadKeyBytes)
	if err != nil {
		return thread.Info{}, err
	}
	_, err = s.threadCreate(archiveId, thread.NewKey(archiveSk, archiveRk))
	if err != nil {
		return thread.Info{}, fmt.Errorf("could not create archive thread: %w", err)
	}

	return workspaceThread, nil
}

func (s *service) SelectWorkspace(
	ctx context.Context,
	ids DerivedSmartblockIds,
	workspaceId thread.ID) (DerivedSmartblockIds, error) {
	return s.ensureWorkspace(ctx, ids, workspaceId, true)
}

func (s *service) AddThread(threadId string, key string, addrs []string) error {
	addedInfo := threadInfo{
		ID:    db.InstanceID(threadId),
		Key:   key,
		Addrs: addrs,
	}
	var err error
	id, err := thread.Decode(threadId)
	if err != nil {
		return fmt.Errorf("failed to add thread: %w", err)
	}

	defer func() {
		// if we successfully downloaded the thread, or we already have it
		// we still may need to check that it is added to current collection
		if err != nil {
			return
		}
		// we shouldn't add references to self
		if s.currentWorkspaceId == id {
			return
		}

		// TODO: check if we can optimize it by changing the query
		instancesBytes, err := s.threadsCollection.Find(&threadsDb.Query{})
		if err != nil {
			log.With("thread id", threadId).
				Errorf("failed to add thread to collection: %v", err)
			return
		}

		for _, instanceBytes := range instancesBytes {
			ti := threadInfo{}
			threadsUtil.InstanceFromJSON(instanceBytes, &ti)

			if string(ti.ID) == threadId {
				return
			}
		}
		_, err = s.threadsCollection.Create(threadsUtil.JSONFromInstance(addedInfo))
		if err != nil {
			log.With("thread id", threadId).
				Errorf("failed to add thread to collection: %v", err)
		}
	}()

	_, err = s.t.GetThread(context.Background(), id)
	if err == nil {
		log.With("thread id", threadId).
			Info("thread was already added")
		return nil
	}

	if err != nil && err != tlcore.ErrThreadNotFound {
		return fmt.Errorf("failed to add thread: %w", err)
	}

	smartBlockType, err := smartblock.SmartBlockTypeFromThreadID(id)
	if smartBlockType == smartblock.SmartBlockTypeWorkspace {
		_, err = s.ensureWorkspace(context.Background(), DerivedSmartblockIds{}, id, true)
	} else {
		err = s.processNewExternalThread(id, addedInfo, true)
	}

	return err
}

func (s *service) GetThreadInfo(id thread.ID) (thread.Info, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	ti, err := s.t.GetThread(ctx, id)
	if err != nil {
		return thread.Info{}, err
	}

	// TODO: consider also getting addresses of logs from thread
	// default thread implementation only returns the addresses of current host
	if s.replicatorAddr != nil {
		ti.Addrs = append(ti.Addrs, s.replicatorAddr)
	}
	return ti, nil
}

func (s *service) CreateThread(blockType smartblock.SmartBlockType) (thread.Info, error) {
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

	key := thread.NewKey(followKey, readKey)

	if s.threadsCollection == nil {
		return thread.Info{}, fmt.Errorf("thread collection not initialized: need to call EnsurePredefinedThreads first")
	}

	// this logic is needed to prevent cases when the tread is created
	// but the app is shut down, so we don't know in which collection should we add this thread
	err = s.threadCreateQueue.AddThreadQueueEntry(&model.ThreadCreateQueueEntry{
		CollectionThread: s.currentWorkspaceId.String(),
		ThreadId:         thrdId.String(),
	})
	if err != nil {
		log.With("thread id", thrdId.String()).
			Errorf("failed to add thread id to queue: %v", err)
	}

	thrd, err := s.t.CreateThread(context.TODO(), thrdId, net.WithThreadKey(key), net.WithLogKey(s.device))
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
	} else {
		err = s.threadCreateQueue.RemoveThreadQueueEntry(thrdId.String())
		if err != nil {
			log.With("thread id", thrdId.String()).
				Errorf("failed to remove thread id to queue: %v", err)
		}
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
