package core

import (
	"context"
	"fmt"
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
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/ftsearch"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/net"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/net/litenet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/libp2p/go-libp2p-core/peer"
	pstore "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	ma "github.com/multiformats/go-multiaddr"
	tcn "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
	tnet "github.com/textileio/go-threads/net"
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

	pullInterval = 3 * time.Minute
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

	FindProfilesByAccountIDs(ctx context.Context, AccountAddrs []string, ch chan Profile) error

	ObjectStore() localstore.ObjectStore
	ObjectInfoWithLinks(id string) (*model.ObjectInfoWithLinks, error)
	ObjectList() ([]*model.ObjectInfo, error)
	ObjectUpdateLastOpened(id string) error

	SyncStatus() tcn.SyncInfo
	FileStatus() FileInfo

	SubscribeForNewRecords() (ch chan SmartblockRecordWithThreadID, cancel func(), err error)
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
	pinRegistry        *filePinRegistry

	logLevels map[string]string

	opts ServiceOptions

	replicationWG    sync.WaitGroup
	migrationOnce    sync.Once
	lock             sync.Mutex
	isStarted        bool          // use under the lock
	shutdownStartsCh chan struct{} // closed when node shutdown starts
	onlineCh         chan struct{} // closed when became online
}

func New(options ...ServiceOption) (*Anytype, error) {
	opts := ServiceOptions{}

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
		pinRegistry:      newFilePinRegistry(),
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

func (a *Anytype) Device() string {
	if a.opts.Device == nil {
		return ""
	}
	return a.opts.Device.Address()
}

func (a *Anytype) SyncStatus() tcn.SyncInfo {
	return a.t
}

func (a *Anytype) FileStatus() FileInfo {
	return a.pinRegistry
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
	err = sb.indexSnapshot(nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to index new block %s: %s", thrd.ID.String(), err.Error())
	}

	return &smartBlock{thread: thrd, node: a}, nil
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

	if a.opts.NetBootstraper != nil {
		a.t = a.opts.NetBootstraper
	} else {
		var err error
		if a.t, err = a.startNetwork(a.opts.HostAddr); err != nil {
			if strings.Contains(err.Error(), "address already in use") { // FIXME proper cross-platform solution?
				// start on random port in case saved port is already used by some other app
				if a.t, err = a.startNetwork(nil); err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	if a.opts.CafeP2PAddr != nil {
		// protect cafe connections from pruning
		if p, err := a.opts.CafeP2PAddr.ValueForProtocol(ma.P_P2P); err == nil {
			if pid, err := peer.Decode(p); err == nil {
				a.t.Host().ConnManager().Protect(pid, "cafe-sync")
			} else {
				log.Errorf("decoding peerID from cafe address failed: %v", err)
			}
		}
	}
	fts, err := ftsearch.NewFTSearch(filepath.Join(a.opts.Repo, "fts"))
	if err != nil {
		log.Errorf("can't start fulltext search service: %v", err)
	}
	a.localStore = localstore.NewLocalStore(a.t.Datastore(), fts)
	a.files = files.New(a.localStore.Files, a.t.GetIpfs(), a.cafe)
	a.threadService = threads.New(a.t, a.t.Logstore(), a.opts.Repo, a.opts.Device, a.opts.Account, func(id thread.ID) error {
		err := a.migratePageToChanges(id)
		if err != nil && err != ErrAlreadyMigrated {
			return err
		}
		return nil
	}, a.opts.NewSmartblockChan, a.opts.CafeP2PAddr)

	// find and retry failed pins
	go a.checkPins()

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

func (a *Anytype) startNetwork(hostAddr ma.Multiaddr) (net.NetBoostrapper, error) {
	var opts = []litenet.NetOption{
		litenet.WithNetHostAddr(hostAddr),
		litenet.WithNetDebug(false),
		litenet.WithOffline(a.opts.Offline),
		litenet.WithInMemoryDS(a.opts.InMemoryDS),
		litenet.WithNetPubSub(true), // TODO control with env var
		litenet.WithNetSyncTracking(),
		litenet.WithNetGRPCServerOptions(
			grpc.MaxRecvMsgSize(1024 * 1024 * 20),
		),
		litenet.WithNetGRPCDialOptions(
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(1024*1024*10),
				grpc.MaxCallSendMsgSize(1024*1024*10),
			),

			// gRPC metrics
			//grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor),
			//grpc.WithStreamInterceptor(grpc_prometheus.StreamClientInterceptor),
		),
	}

	return litenet.DefaultNetwork(a.opts.Repo, a.opts.Device, []byte(ipfsPrivateNetworkKey), opts...)
}

func (a *Anytype) SubscribeForNewRecords() (ch chan SmartblockRecordWithThreadID, cancel func(), err error) {
	var ctx context.Context
	ctx, cancel = context.WithCancel(context.Background())
	ch = make(chan SmartblockRecordWithThreadID)
	threadsCh, err := a.t.Subscribe(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to subscribe: %s", err.Error())
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
				if block, ok = smartBlocksCache[id]; !ok {
					if block, err = a.GetSmartBlock(id); err != nil {
						log.Errorf("failed to open smartblock %s: %v", id, err)
						continue
					} else {
						smartBlocksCache[id] = block
					}
				}
				rec, err := block.decodeRecord(ctx, val.Value())
				if err != nil {
					log.Errorf("failed to decode thread record: %s", err.Error())
					continue
				}
				select {

				case ch <- SmartblockRecordWithThreadID{
					SmartblockRecordWithLogID: SmartblockRecordWithLogID{
						SmartblockRecord: *rec,
						LogID:            val.LogID().String(),
					},
					ThreadID: id,
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

	return ch, cancel, nil
}

func init() {
	// redefine thread pulling interval
	tnet.PullInterval = pullInterval

	// redefine timeouts for threads
	tnet.DialTimeout = 10 * time.Second
	tnet.PushTimeout = 30 * time.Second
	tnet.PullTimeout = 2 * time.Minute

	// apply log levels in go-threads and go-ipfs deps
	logging.ApplyLevelsFromEnv()
}
