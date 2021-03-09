package core

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/libp2p/go-tcp-transport"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/threads"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	fts "github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/net"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/net/litenet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pin"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/libp2p/go-libp2p-core/peer"
	pstore "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	ma "github.com/multiformats/go-multiaddr"
	tcn "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	tnet "github.com/textileio/go-threads/net"
	tq "github.com/textileio/go-threads/net/queue"
	"github.com/textileio/go-threads/util"
	"google.golang.org/grpc"
)

var log = logging.Logger("anytype-core")

const (
	ipfsPrivateNetworkKey = `/key/swarm/psk/1.0.0/
/base16/
fee6e180af8fc354d321fde5c84cab22138f9c62fec0d1bc0e99f4439968b02c`

	keyFileAccount = "account.key"
	keyFileDevice  = "device.key"
)

const (
	DefaultWebGatewaySnapshotURI = "/%s/snapshotId/%s#key=%s"
)

var BootstrapNodes = []string{
	"/ip4/54.93.109.23/tcp/4001/p2p/QmZ4P1Q8HhtKpMshHorM2HDg4iVGZdhZ7YN7WeWDWFH3Hi",           // fra1
	"/dns4/bootstrap2.anytype.io/tcp/4001/p2p/QmSxuiczQTjgj5agSoNtp4esSsj64RisDyKt2MCZQsKZUx", // sfo1
	"/dns4/bootstrap3.anytype.io/tcp/4001/p2p/QmUdDTWzgdcf4cM4aHeihoYSUfQJJbLVLTZFZvm1b46NNT", // sgp1
}

type PredefinedBlockIds struct {
	Account string
	Profile string
	Home    string
	Archive string

	SetPages string
}

type Service interface {
	Account() string
	Device() string
	CafePeer() ma.Multiaddr

	Start() error
	Stop() error
	IsStarted() bool
	BecameOnline(ch chan<- error)

	ThreadService() threads.Service

	// InitNewSmartblocksChan allows to init the chan to inform when there is a new smartblock becomes available
	// Can be called only once. Returns error if called more than once
	InitNewSmartblocksChan(ch chan<- string) error
	InitPredefinedBlocks(ctx context.Context, mustSyncFromRemote bool) error
	PredefinedBlocks() threads.DerivedSmartblockIds
	GetBlock(blockId string) (SmartBlock, error)
	DeleteBlock(blockId string) error
	CreateBlock(t smartblock.SmartBlockType) (SmartBlock, error)

	FileByHash(ctx context.Context, hash string) (File, error)
	FileAdd(ctx context.Context, opts ...files.AddOption) (File, error)
	FileAddWithBytes(ctx context.Context, content []byte, filename string) (File, error)         // deprecated
	FileAddWithReader(ctx context.Context, content io.ReadSeeker, filename string) (File, error) // deprecated
	FileGetKeys(hash string) (*files.FileKeys, error)
	FileStoreKeys(fileKeys ...files.FileKeys) error

	ImageByHash(ctx context.Context, hash string) (Image, error)
	ImageAdd(ctx context.Context, opts ...files.AddOption) (Image, error)
	ImageAddWithBytes(ctx context.Context, content []byte, filename string) (Image, error)         // deprecated
	ImageAddWithReader(ctx context.Context, content io.ReadSeeker, filename string) (Image, error) // deprecated

	ObjectStore() localstore.ObjectStore
	ObjectInfoWithLinks(id string) (*model.ObjectInfoWithLinks, error)
	ObjectList() ([]*model.ObjectInfo, error)

	SyncStatus() tcn.SyncInfo
	FileStatus() pin.FilePinService
	SubscribeForNewRecords(ctx context.Context) (ch chan SmartblockRecordWithThreadID, err error)

	ProfileInfo
}

var _ Service = (*Anytype)(nil)

type Anytype struct {
	t                  net.NetBoostrapper
	files              *files.Service
	cafe               cafe.Client
	mdns               discovery.Service
	localStore         localstore.LocalStore
	predefinedBlockIds threads.DerivedSmartblockIds
	threadService      threads.Service
	pinService         pin.FilePinService

	logLevels map[string]string

	opts ServiceOptions

	replicationWG    sync.WaitGroup
	migrationOnce    sync.Once
	lock             sync.Mutex
	isStarted        bool // use under the lock
	shutdownStartsCh chan struct {
	} // closed when node shutdown starts
	onlineCh chan struct {
	} // closed when became online
}

func New(options ...ServiceOption) (*Anytype, error) {
	var opts ServiceOptions
	for _, opt := range options {
		err := opt(&opts)
		if err != nil {
			return nil, err
		}
	}

	if opts.Device == nil {
		return nil, fmt.Errorf("no device keypair provided")
	}

	logging.SetHost(opts.Device.Address())

	a := &Anytype{
		opts:             opts,
		shutdownStartsCh: make(chan struct{}),
		onlineCh:         make(chan struct{}),
	}

	if opts.CafeGrpcHost != "" {
		isLocal := strings.HasPrefix(opts.CafeGrpcHost, "127.0.0.1") || strings.HasPrefix(opts.CafeGrpcHost, "localhost")

		var err error
		a.cafe, err = cafe.NewClient(opts.CafeGrpcHost, "<todo>", isLocal, opts.Device, opts.Account)
		if err != nil {
			return nil, fmt.Errorf("failed to get grpc client: %w", err)
		}
	}

	return a, nil
}

func (a *Anytype) Account() string {
	if a.opts.Account == nil {
		return ""
	}
	return a.opts.Account.Address()
}

func (a *Anytype) CafePeer() ma.Multiaddr {
	if a.opts.CafeP2PAddr == nil {
		return nil
	}

	return a.opts.CafeP2PAddr
}

func (a *Anytype) Device() string {
	if a.opts.Device == nil {
		return ""
	}
	return a.opts.Device.Address()
}

func (a *Anytype) SyncStatus() tcn.SyncInfo {
	return a.t
}

func (a *Anytype) FileStatus() pin.FilePinService {
	return a.pinService
}

func (a *Anytype) Ipfs() ipfs.IPFS {
	return a.t.GetIpfs()
}

func (a *Anytype) IsStarted() bool {
	a.lock.Lock()
	defer a.lock.Unlock()

	return a.isStarted
}

func (a *Anytype) BecameOnline(ch chan<- error) {
	for {
		select {
		case <-a.onlineCh:
			ch <- nil
			close(ch)
		case <-a.shutdownStartsCh:
			ch <- fmt.Errorf("node was shutdown")
			close(ch)
		}
	}
}

func (a *Anytype) CreateBlock(t smartblock.SmartBlockType) (SmartBlock, error) {
	thrd, err := a.threadService.CreateThread(t)
	if err != nil {
		return nil, err
	}
	sb := &smartBlock{thread: thrd, node: a}
	return sb, nil
}

// PredefinedBlocks returns default blocks like home and archive
// ⚠️ Will return empty struct in case it runs before Anytype.Start()
func (a *Anytype) PredefinedBlocks() threads.DerivedSmartblockIds {
	return a.predefinedBlockIds
}

func (a *Anytype) HandlePeerFound(p peer.AddrInfo) {
	a.t.Host().Peerstore().AddAddrs(p.ID, p.Addrs, pstore.ConnectedAddrTTL)
}

func (a *Anytype) runPeriodicJobsInBackground() {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()

	go func() {
		for {
			select {
			case <-tick.C:
				//a.syncAccount(false)

			case <-a.shutdownStartsCh:
				return
			}
		}
	}()
}

func DefaultBoostrapPeers() []peer.AddrInfo {
	ais, err := util.ParseBootstrapPeers(BootstrapNodes)
	if err != nil {
		panic("coudn't parse default bootstrap peers")
	}
	return ais
}

func (a *Anytype) Start() error {
	err := a.RunMigrations()
	if err != nil {
		return err
	}

	return a.start()
}

func (a *Anytype) start() error {
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.isStarted {
		return nil
	}

	var err error
	if a.opts.NetBootstraper != nil {
		a.t = a.opts.NetBootstraper
	} else {
		if a.t, err = a.startNetwork(true); err != nil {
			if strings.Contains(err.Error(), "address already in use") { // FIXME proper cross-platform solution?
				// start on random port in case saved port is already used by some other app
				if a.t, err = a.startNetwork(false); err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	var fullTextSearch fts.FTSearch
	if a.opts.FullTextSearch {
		fullTextSearch, err = fts.NewFTSearch(filepath.Join(a.opts.Repo, "fts"))
		if err != nil {
			log.Errorf("can't start fulltext search service: %v", err)
		}
	}

	a.localStore = localstore.NewLocalStore(a.t.Datastore(), fullTextSearch)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { <-a.shutdownStartsCh; cancel() }()
	a.pinService = pin.NewFilePinService(ctx, a.cafe, a.localStore.Files)

	if a.opts.CafeP2PAddr != nil {
		// protect cafe connections from pruning
		if p, err := a.opts.CafeP2PAddr.ValueForProtocol(ma.P_P2P); err == nil {
			if pid, err := peer.Decode(p); err == nil {
				a.t.Host().ConnManager().Protect(pid, "cafe-sync")
			} else {
				log.Errorf("decoding peerID from cafe address failed: %v", err)
			}
		}
		// start syncing with cafe
		a.pinService.Start()
	}

	a.files = files.New(a.localStore.Files, a.t.GetIpfs(), a.pinService)
	a.threadService = threads.New(
		a.t,
		a.t.Logstore(),
		a.opts.Repo,
		a.opts.Device,
		a.opts.Account,
		func(id thread.ID) error {
			err := a.migratePageToChanges(id)
			if err != nil && err != ErrAlreadyMigrated {
				return err
			}
			return nil
		},
		a.opts.NewSmartblockChan,
		a.opts.CafeP2PAddr,
	)

	go func(net net.NetBoostrapper, offline bool, onlineCh chan struct{}) {
		if offline {
			return
		}

		net.Bootstrap(DefaultBoostrapPeers())
		// todo: init mdns discovery and register notifee
		// discovery.NewMdnsService
		close(onlineCh)
	}(a.t, a.opts.Offline, a.onlineCh)

	log.Info("Anytype device: " + a.opts.Device.Address())
	log.Info("Anytype account: " + a.Account())

	a.isStarted = true
	return nil
}

func (a *Anytype) InitPredefinedBlocks(ctx context.Context, accountSelect bool) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		select {
		case <-cctx.Done():
			return
		case <-a.shutdownStartsCh:
			cancel()
		}
	}()

	ids, err := a.threadService.EnsurePredefinedThreads(cctx, !accountSelect)
	if err != nil {
		return err
	}

	a.predefinedBlockIds = ids

	//a.runPeriodicJobsInBackground()
	return nil
}

func (a *Anytype) Stop() error {
	fmt.Printf("stopping the library...\n")
	defer fmt.Println("library has been successfully stopped")
	a.lock.Lock()
	defer a.lock.Unlock()
	a.isStarted = false

	if a.shutdownStartsCh != nil {
		close(a.shutdownStartsCh)
	}

	// fixme useless!
	a.replicationWG.Wait()

	if a.mdns != nil {
		err := a.mdns.Close()
		if err != nil {
			return err
		}
	}

	if a.t != nil {
		err := a.t.Close()
		if err != nil {
			return err
		}
	}

	if a.threadService != nil {
		err := a.threadService.Close()
		if err != nil {
			return err
		}
	}

	err := a.localStore.Close()
	if err != nil {
		return err
	}

	return nil
}

func (a *Anytype) ThreadService() threads.Service {
	return a.threadService
}

func (a *Anytype) InitNewSmartblocksChan(ch chan<- string) error {
	if a.threadService == nil {
		return fmt.Errorf("thread service not ready yet")
	}

	return a.threadService.InitNewThreadsChan(ch)
}

func (a *Anytype) startNetwork(useHostAddr bool) (net.NetBoostrapper, error) {
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

	var opts = []litenet.NetOption{
		litenet.WithInMemoryDS(a.opts.InMemoryDS),
		litenet.WithOffline(a.opts.Offline),
		litenet.WithNetDebug(a.opts.NetDebug),
		litenet.WithNetPubSub(a.opts.NetPubSub),
		litenet.WithNetSyncTracking(a.opts.SyncTracking),
		litenet.WithNetGRPCServerOptions(
			grpc.MaxRecvMsgSize(5<<20), // 5Mb max message size
			grpc.UnaryInterceptor(unaryServerInterceptor),
		),
		litenet.WithNetGRPCDialOptions(
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(5<<20),
				grpc.MaxCallSendMsgSize(5<<20),
			),
			grpc.WithUnaryInterceptor(unaryClientInterceptor),
		),
	}

	if useHostAddr {
		opts = append(opts, litenet.WithNetHostAddr(a.opts.HostAddr))
	}

	return litenet.DefaultNetwork(a.opts.Repo, a.opts.Device, []byte(ipfsPrivateNetworkKey), opts...)
}

func (a *Anytype) SubscribeForNewRecords(ctx context.Context) (ch chan SmartblockRecordWithThreadID, err error) {
	ctx, cancel := context.WithCancel(ctx)
	ch = make(chan SmartblockRecordWithThreadID)
	threadsCh, err := a.t.Subscribe(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %s", err.Error())
	}

	go func() {
		smartBlocksCache := make(map[string]*smartBlock)
		defer close(ch)
		for {
			select {
			case val, ok := <-threadsCh:
				if !ok {
					return
				}
				var block *smartBlock
				id := val.ThreadID().String()
				if id == a.predefinedBlockIds.Account {
					continue
				}
				if block, ok = smartBlocksCache[id]; !ok {
					if block, err = a.GetSmartBlock(id); err != nil {
						log.Errorf("failed to open smartblock %s: %v", id, err)
						continue
					} else {
						smartBlocksCache[id] = block
					}
				}
				rec, err := block.decodeRecord(ctx, val.Value(), true)
				if err != nil {
					log.Errorf("failed to decode thread record: %s", err.Error())
					continue
				}
				select {

				case ch <- SmartblockRecordWithThreadID{
					SmartblockRecordEnvelope: *rec,
					ThreadID:                 id,
				}:
					// everything is ok
				case <-ctx.Done():
					// no need to cancel, continue to read the rest msgs from the channel
					continue
				case <-a.shutdownStartsCh:
					// cancel first, then we should read ok == false from the threadsCh
					cancel()
				}
			case <-ctx.Done():
				continue
			case <-a.shutdownStartsCh:
				cancel()
			}
		}
	}()

	return ch, nil
}

func init() {
	/* adjust ThreadsDB parameters */

	// thread pulling cycle
	tnet.PullStartAfter = 5 * time.Second
	tnet.InitialPullInterval = 20 * time.Second
	tnet.PullInterval = 3 * time.Minute

	// communication timeouts
	tnet.DialTimeout = 20 * time.Second // we can set safely set a long dial timeout because unavailable peer are cached for some time and local network timeouts are overridden with 5s
	tcp.DefaultConnectTimeout = tnet.DialTimeout // override default tcp dial timeout because it has a priority over the passing context's deadline
	tnet.PushTimeout = 30 * time.Second
	tnet.PullTimeout = 2 * time.Minute

	// event bus input buffer
	tnet.EventBusCapacity = 3

	// exchange edges
	tnet.MaxThreadsExchanged = 10
	tnet.ExchangeCompressionTimeout = 20 * time.Second
	tnet.QueuePollInterval = 1 * time.Second

	// thread packer queue
	tq.InBufSize = 5
	tq.OutBufSize = 2

	/* logs */

	// apply log levels in go-threads and go-ipfs deps
	logging.ApplyLevelsFromEnv()
}
