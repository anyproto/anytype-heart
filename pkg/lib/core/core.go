package core

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/cafe"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/config"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/threads"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/net"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/net/litenet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/wallet"
	"github.com/gogo/protobuf/proto"
	"github.com/gogo/protobuf/types"
	"github.com/libp2p/go-libp2p-core/peer"
	pstore "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	"github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-threads/core/thread"
	net2 "github.com/textileio/go-threads/net"
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

type Anytype struct {
	t                  net.NetBoostrapper
	files              *files.Service
	cafe               cafe.Client
	mdns               discovery.Service
	localStore         localstore.LocalStore
	predefinedBlockIds threads.DerivedSmartblockIds
	threadService      threads.Service

	logLevels map[string]string

	opts ServiceOptions

	replicationWG    sync.WaitGroup
	migrationOnce    sync.Once
	lock             sync.Mutex
	isStarted        bool          // use under the lock
	shutdownStartsCh chan struct{} // closed when node shutdown starts
	onlineCh         chan struct{} // closed when became online
}

type Service interface {
	Account() string
	Device() string

	Start() error
	Stop() error
	IsStarted() bool
	BecameOnline(ch chan<- error)

	ThreadService() threads.Service

	InitPredefinedBlocks(ctx context.Context, mustSyncFromRemote bool) error
	PredefinedBlocks() threads.DerivedSmartblockIds
	GetBlock(blockId string) (SmartBlock, error)
	DeleteBlock(blockId string) error
	CreateBlock(t smartblock.SmartBlockType) (SmartBlock, error)

	FileByHash(ctx context.Context, hash string) (File, error)
	FileAdd(ctx context.Context, opts ...files.AddOption) (File, error)
	FileAddWithBytes(ctx context.Context, content []byte, filename string) (File, error)     // deprecated
	FileAddWithReader(ctx context.Context, content io.Reader, filename string) (File, error) // deprecated
	FileGetKeys(hash string) (*FileKeys, error)
	FileStoreKeys(fileKeys ...FileKeys) error

	ImageByHash(ctx context.Context, hash string) (Image, error)
	ImageAdd(ctx context.Context, opts ...files.AddOption) (Image, error)
	ImageAddWithBytes(ctx context.Context, content []byte, filename string) (Image, error)     // deprecated
	ImageAddWithReader(ctx context.Context, content io.Reader, filename string) (Image, error) // deprecated

	FindProfilesByAccountIDs(ctx context.Context, AccountAddrs []string, ch chan Profile) error

	ObjectStore() localstore.ObjectStore
	PageInfoWithLinks(id string) (*model.PageInfoWithLinks, error)
	PageList() ([]*model.PageInfo, error)
	PageUpdateLastOpened(id string) error
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

func (a *Anytype) ThreadsNet() net.NetBoostrapper {
	return a.t
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
	err = sb.indexSnapshot(nil, nil)
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

func init() {
	net2.PullInterval = pullInterval
	// apply log levels in go-threads and go-ipfs deps
	logging.ApplyLevelsFromEnv()
}

func NewFromOptions(options ...ServiceOption) (*Anytype, error) {
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
		opts:          opts,
		replicationWG: sync.WaitGroup{},
		migrationOnce: sync.Once{},

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

func New(rootPath string, account string, reIndexFunc func(id string) error, snapshotMarshalerFunc func(blocks []*model.Block, details *types.Struct, relations []*pbrelation.Relation, fileKeys []*FileKeys) proto.Marshaler) (Service, error) {
	opts, err := getNewConfig(rootPath, account)
	if err != nil {
		return nil, err
	}

	opts = append(opts, WithReindexFunc(reIndexFunc), WithSnapshotMarshalerFunc(snapshotMarshalerFunc))
	return NewFromOptions(opts...)
}

func getNewConfig(rootPath string, account string) ([]ServiceOption, error) {
	repoPath := filepath.Join(rootPath, account)

	b, err := ioutil.ReadFile(filepath.Join(repoPath, keyFileAccount))
	if err != nil {
		return nil, fmt.Errorf("failed to read account keyfile: %w", err)
	}

	accountKp, err := wallet.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}
	if accountKp.KeypairType() != wallet.KeypairTypeAccount {
		return nil, fmt.Errorf("got %s key type instead of %s", accountKp.KeypairType(), wallet.KeypairTypeAccount)
	}

	b, err = ioutil.ReadFile(filepath.Join(repoPath, keyFileDevice))
	if err != nil {
		return nil, fmt.Errorf("failed to read device keyfile: %w", err)
	}

	deviceKp, err := wallet.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}

	if deviceKp.KeypairType() != wallet.KeypairTypeDevice {
		return nil, fmt.Errorf("got %s key type instead of %s", deviceKp.KeypairType(), wallet.KeypairTypeDevice)
	}

	cfg, err := config.GetConfig(repoPath)
	if err != nil {
		return nil, err
	}

	opts := []ServiceOption{WithRepo(repoPath), WithDeviceKey(deviceKp), WithAccountKey(accountKp), WithHostMultiaddr(cfg.HostAddr), WithWebGatewayBaseUrl(cfg.WebGatewayBaseUrl)}

	// "-" or any other single char assumes as empty for env var compatability
	if len(cfg.CafeP2PAddr) > 1 {
		opts = append(opts, WithCafeP2PAddr(cfg.CafeP2PAddr))
	}

	if len(cfg.CafeGRPCAddr) > 1 {
		opts = append(opts, WithCafeGRPCHost(cfg.CafeGRPCAddr))
	}

	return opts, nil
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
		if a.t, err = a.startNetwork(a.opts.HostAddr, a.opts.Offline); err != nil {
			if strings.Contains(err.Error(), "address already in use") { // FIXME proper cross-platform solution?
				// start on random port in case saved port is already used by some other app
				if a.t, err = a.startNetwork(nil, a.opts.Offline); err != nil {
					return err
				}
			} else {
				return err
			}
		}
	}

	a.localStore = localstore.NewLocalStore(a.t.Datastore())
	a.files = files.New(a.localStore.Files, a.t.GetIpfs(), a.cafe)
	a.threadService = threads.New(a.t, a.t.Logstore(), a.opts.Repo, a.opts.Device, a.opts.Account, func(id thread.ID) error {
		err := a.migratePageToChanges(id)
		if err != nil && err != ErrAlreadyMigrated {
			return err
		}
		go func() {
			// todo: mw is locked during AccountSelect, this leads to deadlock in doBlockService
			// as a workaround,do it in the goroutine
			err = a.opts.ReindexFunc(id.String())
			if err != nil {
				log.Errorf("ReindexFunc failed: %s", err.Error())
			}
		}()
		return nil
	}, a.opts.CafeP2PAddr)

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

func (a *Anytype) startNetwork(hostAddr multiaddr.Multiaddr, offline bool) (net.NetBoostrapper, error) {
	return litenet.DefaultNetwork(
		a.opts.Repo,
		a.opts.Device,
		[]byte(ipfsPrivateNetworkKey),
		litenet.WithNetHostAddr(hostAddr),
		litenet.WithNetDebug(false),
		litenet.WithOffline(offline),
		litenet.WithNetPubSub(true), // TODO control with env var
		litenet.WithNetGRPCServerOptions(
			grpc.MaxRecvMsgSize(1024*1024*20),
		),
		litenet.WithNetGRPCDialOptions(
			grpc.WithDefaultCallOptions(
				grpc.MaxCallRecvMsgSize(1024*1024*10),
				grpc.MaxCallSendMsgSize(1024*1024*10),
			),

			// TODO metrics
			//grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor),
			//grpc.WithStreamInterceptor(grpc_prometheus.StreamClientInterceptor),
		),
	)
}

func init() {
	// redefine timeouts
	net2.DialTimeout = 10 * time.Second
	net2.PushTimeout = 30 * time.Second
	net2.PullTimeout = 2 * time.Minute
}
