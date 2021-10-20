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

// TODO: remove when workspace debugging ends
var WorkspaceLogger = logging.Logger("anytype-workspace-debug")
var ErrCreatorInfoNotFound = fmt.Errorf("no creator info in collection")

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
	objectDeleter         ObjectDeleter
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
	s.objectDeleter = a.MustComponent("blockService").(ObjectDeleter)

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

	CreateWorkspace(string) (thread.Info, error)
	SelectWorkspace(ctx context.Context, workspaceId thread.ID) error
	SetIsHighlighted(workspaceId, objectId string, isHighlighted bool) error
	SelectAccount() error
	CreateThread(blockType smartblock.SmartBlockType, workspaceId string) (thread.Info, error)
	DeleteThread(id, workspace string) error
	InitNewThreadsChan(ch chan<- string) error // can be called only once

	GetAllWorkspaces() ([]string, error)
	GetAllThreadsInWorkspace(id string) ([]string, error)
	GetLatestWorkspaceMeta(workspaceId string) (WorkspaceMeta, error)
	GetThreadProcessorForWorkspace(id string) (ThreadProcessor, error)
	AddCreatorInfoToWorkspace(workspaceId string) error
	GetCreatorInfoForWorkspace(workspaceId string) (CreatorInfo, error)

	GetThreadInfo(id thread.ID) (thread.Info, error)
	AddThread(threadId string, key string, addrs []string) error

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

func (s *service) GetAllWorkspaces() ([]string, error) {
	threads, err := s.logstore.Threads()
	if err != nil {
		return nil, fmt.Errorf("could not get all workspace threads: %w", err)
	}

	var workspaceThreads []string
	for _, th := range threads {
		if tp, err := smartblock.SmartBlockTypeFromThreadID(th); err == nil && tp == smartblock.SmartBlockTypeWorkspace {
			workspaceThreads = append(workspaceThreads, th.String())
		}
	}
	return workspaceThreads, nil
}

func (s *service) GetAllThreadsInWorkspace(id string) ([]string, error) {
	threadId, err := thread.Decode(id)
	if err != nil {
		return nil, err
	}

	s.processorMutex.RLock()
	processor, exists := s.threadProcessors[threadId]
	s.processorMutex.RUnlock()

	if !exists {
		processor, err = s.startWorkspaceThreadProcessor(id)
		if err != nil {
			return nil, err
		}
	}

	collection := processor.GetThreadCollection()
	instancesBytes, err := collection.Find(&threadsDb.Query{})
	if err != nil {
		return nil, err
	}

	var threadsInWorkspace []string
	for _, instanceBytes := range instancesBytes {
		ti := threadInfo{}
		threadsUtil.InstanceFromJSON(instanceBytes, &ti)

		tid, err := thread.Decode(ti.ID.String())
		if err != nil {
			continue
		}
		threadsInWorkspace = append(threadsInWorkspace, tid.String())
	}

	return threadsInWorkspace, nil
}

func (s *service) GetLatestWorkspaceMeta(workspaceId string) (WorkspaceMeta, error) {
	threadId, err := thread.Decode(workspaceId)
	if err != nil {
		return nil, err
	}

	s.processorMutex.RLock()
	processor, exists := s.threadProcessors[threadId]
	s.processorMutex.RUnlock()

	if !exists {
		processor, err = s.startWorkspaceThreadProcessor(workspaceId)
		if err != nil {
			return nil, err
		}
	}

	metaCollection := processor.GetCollectionWithPrefix(MetaCollectionName)

	results, err := metaCollection.Find(&threadsDb.Query{})
	if err != nil {
		return nil, fmt.Errorf("could not get meta for workspace: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no meta entries found in workspace")
	}

	mInfo := MetaInfo{}
	threadsUtil.InstanceFromJSON(results[0], &mInfo)

	return &mInfo, nil
}

func (s *service) AddCreatorInfoToWorkspace(workspaceId string) error {
	deviceId := s.device.Address()
	_, err := s.GetCreatorInfoForWorkspace(workspaceId)
	if err == nil {
		return nil
	}

	processor, err := s.GetThreadProcessorForWorkspace(workspaceId)
	if err != nil {
		return err
	}

	creatorCollection := processor.GetCollectionWithPrefix(CreatorCollectionName)
	if creatorCollection == nil {
		return fmt.Errorf("workspace doesn't have creator collection")
	}

	profileId, err := ProfileThreadIDFromAccountAddress(s.account.Address())
	if err != nil {
		return err
	}
	info, err := s.GetThreadInfo(profileId)
	if err != nil {
		return err
	}

	signature, err := s.account.Sign([]byte(workspaceId + deviceId))
	if err != nil {
		return fmt.Errorf("cannot sign device and workspace")
	}
	creator := CreatorInfo{
		ID:            db.InstanceID(deviceId),
		AccountPubKey: s.account.Address(),
		WorkspaceSig:  signature,
		Addrs:         util.MultiAddressesToStrings(info.Addrs),
	}
	_, err = creatorCollection.Create(threadsUtil.JSONFromInstance(creator))
	return err
}

func (s *service) GetCreatorInfoForWorkspace(workspaceId string) (CreatorInfo, error) {
	deviceId := s.device.Address()
	processor, err := s.GetThreadProcessorForWorkspace(workspaceId)
	if err != nil {
		return CreatorInfo{}, err
	}

	creatorCollection := processor.GetCollectionWithPrefix(CreatorCollectionName)
	if creatorCollection == nil {
		return CreatorInfo{}, fmt.Errorf("workspace doesn't have creator collection")
	}
	result, err := creatorCollection.FindByID(db.InstanceID(deviceId))
	if err != nil {
		return CreatorInfo{}, ErrCreatorInfoNotFound
	}

	var info CreatorInfo
	threadsUtil.InstanceFromJSON(result, &info)

	return info, nil
}

func (s *service) SetIsHighlighted(workspaceId, objectId string, isHighlighted bool) error {
	threadId, err := thread.Decode(workspaceId)
	if err != nil {
		return err
	}

	s.processorMutex.RLock()
	processor, exists := s.threadProcessors[threadId]
	s.processorMutex.RUnlock()

	if !exists {
		processor, err = s.startWorkspaceThreadProcessor(workspaceId)
		if err != nil {
			return err
		}
	}

	collection := processor.GetCollectionWithPrefix(HighlightedCollectionName)
	if collection == nil {
		return fmt.Errorf("no highlighted collection")
	}

	info := CollectionUpdateInfo{
		ID:    db.InstanceID(objectId),
		Value: struct{}{},
	}

	if isHighlighted {
		err = collection.Save(threadsUtil.JSONFromInstance(info))
	} else {
		err = collection.Delete(db.InstanceID(objectId))
	}

	if err != nil {
		WorkspaceLogger.
			With("title object", objectId).
			With("workspace id", workspaceId).
			Errorf("failed to set isHighlighted: %v", err)
	} else {
		WorkspaceLogger.
			With("title object", objectId).
			With("workspace id", workspaceId).
			Debug("setting isHighlighted succeeded")
	}
	return err
}

func (s *service) GetThreadProcessorForWorkspace(id string) (ThreadProcessor, error) {
	threadId, err := thread.Decode(id)
	if err != nil {
		return nil, err
	}

	s.processorMutex.RLock()
	processor, exists := s.threadProcessors[threadId]
	s.processorMutex.RUnlock()

	if !exists {
		processor, err = s.startWorkspaceThreadProcessor(id)
		if err != nil {
			return nil, err
		}
	}

	return processor, nil
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

func (s *service) CreateWorkspace(name string) (thread.Info, error) {
	accountProcessor, err := s.getAccountProcessor()
	if err != nil {
		return thread.Info{}, err
	}

	// create new workspace thread
	workspaceThread, err := s.createThreadWithCollection(
		smartblock.SmartBlockTypeWorkspace,
		accountProcessor.GetThreadCollection(),
		accountProcessor.GetThreadId())
	if err != nil {
		return thread.Info{}, fmt.Errorf("failed to create new workspace thread: %w", err)
	}

	WorkspaceLogger.
		With("workspace id", workspaceThread.ID.String()).
		With("name", name).
		Debug("trying to create workspace")

	processor, err := s.startWorkspaceThreadProcessor(workspaceThread.ID.String())
	if err != nil {
		return thread.Info{}, fmt.Errorf("could not start thread processor: %w", err)
	}

	mInfo := MetaInfo{
		ID:            db.NewInstanceID(),
		Name:          name,
		AccountPubKey: s.account.Address(),
	}
	metaCollection := processor.GetCollectionWithPrefix(MetaCollectionName)
	_, err = metaCollection.Create(threadsUtil.JSONFromInstance(mInfo))
	if err != nil {
		return thread.Info{}, fmt.Errorf("could not create workspace: %w", err)
	}

	err = s.AddCreatorInfoToWorkspace(workspaceThread.ID.String())
	if err != nil {
		return thread.Info{}, nil
	}

	workspaceReadKeyBytes := workspaceThread.Key.Read().Bytes()

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

	WorkspaceLogger.
		With("workspace id", workspaceThread.ID.String()).
		With("name", name).
		Debug("created workspace")
	return workspaceThread, nil
}

func (s *service) SelectWorkspace(
	ctx context.Context,
	workspaceId thread.ID) error {
	return s.ensureWorkspace(ctx, workspaceId, true, true)
}

func (s *service) SelectAccount() error {
	accountProcessor, err := s.getAccountProcessor()
	if err != nil {
		return err
	}

	// TODO: we should probably add some mutex here to prevent concurrent changes
	s.threadsCollection = accountProcessor.GetThreadCollection()
	s.db = accountProcessor.GetDB()
	s.currentWorkspaceId = accountProcessor.GetThreadId()

	WorkspaceLogger.
		With("collection name", s.threadsCollection.GetName()).
		With("account id", s.currentWorkspaceId).
		Debug("switching to account")

	return nil
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

	collectionToAdd := s.threadsCollection

	defer func() {
		// if we successfully downloaded the thread, or we already have it
		// we still may need to check that it is added to current collection
		if err != nil {
			return
		}

		// TODO: check if we can optimize it by changing the query
		instancesBytes, err := collectionToAdd.Find(&threadsDb.Query{})
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
		_, err = collectionToAdd.Create(threadsUtil.JSONFromInstance(addedInfo))
		if err != nil {
			log.With("thread id", threadId).
				Errorf("failed to add thread to collection: %v", err)
		}

		WorkspaceLogger.
			With("thread id", threadId).
			With("collection name", collectionToAdd.GetName()).
			Debug("adding thread to collection")
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

	err = s.processNewExternalThread(id, addedInfo, true)
	if err != nil {
		return err
	}

	smartBlockType, err := smartblock.SmartBlockTypeFromThreadID(id)
	if smartBlockType == smartblock.SmartBlockTypeWorkspace {
		accountProcessor, err := s.getAccountProcessor()
		if err != nil {
			return err
		}

		collectionToAdd = accountProcessor.GetThreadCollection()

		err = s.ensureWorkspace(context.Background(), id, true, false)
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

func (s *service) CreateThread(blockType smartblock.SmartBlockType, workspaceId string) (thread.Info, error) {
	var err error
	insertedWorkspaceId := s.currentWorkspaceId
	if workspaceId != "" {
		insertedWorkspaceId, err = thread.Decode(workspaceId)
		if err != nil {
			return thread.Info{}, fmt.Errorf("could not create thread, because workspace id could not be decoded: %w", err)
		}
	}

	s.processorMutex.RLock()
	processor, exists := s.threadProcessors[insertedWorkspaceId]
	s.processorMutex.RUnlock()

	if !exists {
		return thread.Info{}, fmt.Errorf("account thread processor does not exist")
	}

	WorkspaceLogger.
		With("workspace/account id", insertedWorkspaceId.String()).
		Debug("creating new thread with workspace or account")

	return s.createThreadWithCollection(blockType, processor.GetThreadCollection(), insertedWorkspaceId)
}

func (s *service) createThreadWithCollection(
	blockType smartblock.SmartBlockType,
	collection *threadsDb.Collection,
	workspaceId thread.ID) (thread.Info, error) {
	if collection == nil {
		return thread.Info{}, fmt.Errorf("collection not initialized")
	}

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

	// this logic is needed to prevent cases when the tread is created
	// but the app is shut down, so we don't know in which collection should we add this thread
	err = s.threadCreateQueue.AddThreadQueueEntry(&model.ThreadCreateQueueEntry{
		CollectionThread: workspaceId.String(),
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

	WorkspaceLogger.
		With("collection name", collection.GetName()).
		With("thread id", thrd.ID.String()).
		Debug("pushing thread to thread collection")

	// todo: wait for threadsCollection to push?
	_, err = collection.Create(threadsUtil.JSONFromInstance(threadInfo))
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

func (s *service) DeleteThread(id, workspace string) error {
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

	var collectionToRemove *threadsDb.Collection
	if workspace == "" {
		collectionToRemove = s.threadsCollection
	} else {
		processor, err := s.GetThreadProcessorForWorkspace(workspace)
		if err != nil {
			return err
		}
		collectionToRemove = processor.GetCollectionWithPrefix(ThreadInfoCollectionName)
		if collectionToRemove == nil {
			return fmt.Errorf("no workspace collection found")
		}
	}
	err = collectionToRemove.Delete(db.InstanceID(id))
	if err != nil {
		log.With("workspace", workspace).With("thread", id).Errorf("failed to remove thread from collection")
	}
	return nil
}

func (s *service) getAccountProcessor() (ThreadProcessor, error) {
	id, err := s.derivedThreadIdByIndex(threadDerivedIndexAccount)
	if err != nil {
		return nil, err
	}

	return s.GetThreadProcessorForWorkspace(id.String())
}
