package core

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"path/filepath"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/cafe"
	"github.com/anytypeio/go-anytype-library/files"
	"github.com/anytypeio/go-anytype-library/ipfs"
	"github.com/anytypeio/go-anytype-library/localstore"
	"github.com/anytypeio/go-anytype-library/logging"
	"github.com/anytypeio/go-anytype-library/net"
	"github.com/anytypeio/go-anytype-library/net/litenet"
	"github.com/anytypeio/go-anytype-library/wallet"
	"github.com/libp2p/go-libp2p-core/peer"
	pstore "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-threads/broadcast"
	"github.com/textileio/go-threads/db"
	net2 "github.com/textileio/go-threads/net"
	"github.com/textileio/go-threads/util"
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
	DefaultHostAddr              = "/ip4/0.0.0.0/tcp/4006"
	DefaultCafeNodeP2P           = "/dns4/cafe1.anytype.io/tcp/4001/p2p/12D3KooWKwPC165PptjnzYzGrEs7NSjsF5vvMmxmuqpA2VfaBbLw"
	DefaultCafeNodeGRPC          = "cafe1.anytype.io:3006"
	DefaultWebGatewayBaseUrl     = "https://anytype.page"
	DefaultWebGatewaySnapshotURI = "/%s/snapshotId/%s#key=%s"
)

var BootstrapNodes = []string{
	"/ip4/161.35.18.3/tcp/4001/p2p/QmZ4P1Q8HhtKpMshHorM2HDg4iVGZdhZ7YN7WeWDWFH3Hi",            // fra1
	"/dns4/bootstrap2.anytype.io/tcp/4001/p2p/QmSxuiczQTjgj5agSoNtp4esSsj64RisDyKt2MCZQsKZUx", // sfo1
	"/dns4/bootstrap3.anytype.io/tcp/4001/p2p/QmUdDTWzgdcf4cM4aHeihoYSUfQJJbLVLTZFZvm1b46NNT", // sgp1
}

type PredefinedBlockIds struct {
	Account string
	Profile string
	Home    string
	Archive string
}

type Anytype struct {
	t                  net.NetBoostrapper
	files              *files.Service
	cafe               cafe.Client
	mdns               discovery.Service
	localStore         localstore.LocalStore
	predefinedBlockIds PredefinedBlockIds
	db                 *db.DB
	threadsCollection  *db.Collection

	account                wallet.Keypair
	device                 wallet.Keypair
	logLevels              map[string]string
	repoPath               string
	cafeP2PAddr            ma.Multiaddr
	cafeGatewayBaseUrl     string
	cafeGatewaySnapshotUri string

	smartBlockChanges *broadcast.Broadcaster
	replicationWG     sync.WaitGroup
	lock              sync.Mutex
	shutdownCh        chan struct{} // closed when node shutdown finishes
	onlineCh          chan struct{} // closed when became online
}

type Service interface {
	Account() string
	Start() error
	Stop() error
	IsStarted() bool
	BecameOnline(ch chan<- error)

	InitPredefinedBlocks(mustSyncFromRemote bool) error
	PredefinedBlocks() PredefinedBlockIds
	GetBlock(blockId string) (SmartBlock, error)
	DeleteBlock(blockId string) error
	CreateBlock(t SmartBlockType) (SmartBlock, error)

	FileByHash(ctx context.Context, hash string) (File, error)
	FileAdd(ctx context.Context, opts ...files.AddOption) (File, error)
	FileAddWithBytes(ctx context.Context, content []byte, filename string) (File, error)     // deprecated
	FileAddWithReader(ctx context.Context, content io.Reader, filename string) (File, error) // deprecated

	ImageByHash(ctx context.Context, hash string) (Image, error)
	ImageAdd(ctx context.Context, opts ...files.AddOption) (Image, error)
	ImageAddWithBytes(ctx context.Context, content []byte, filename string) (Image, error)     // deprecated
	ImageAddWithReader(ctx context.Context, content io.Reader, filename string) (Image, error) // deprecated

	FindProfilesByAccountIDs(ctx context.Context, AccountAddrs []string, ch chan Profile) error
}

func (a *Anytype) Account() string {
	return a.account.Address()
}

func (a *Anytype) Ipfs() ipfs.IPFS {
	return a.t.GetIpfs()
}

func (a *Anytype) IsStarted() bool {
	return a.t != nil && a.t.GetIpfs() != nil
}

func (a *Anytype) BecameOnline(ch chan<- error) {
	for {
		select {
		case <-a.onlineCh:
			ch <- nil
			close(ch)
		case <-a.shutdownCh:
			ch <- fmt.Errorf("node was shutdown")
			close(ch)
		}
	}
}

func (a *Anytype) CreateBlock(t SmartBlockType) (SmartBlock, error) {
	thrd, err := a.newBlockThread(t)
	if err != nil {
		return nil, err
	}

	return &smartBlock{thread: thrd, node: a}, nil
}

// PredefinedBlocks returns default blocks like home and archive
// ⚠️ Will return empty struct in case it runs before Anytype.Start()
func (a *Anytype) PredefinedBlocks() PredefinedBlockIds {
	return a.predefinedBlockIds
}

func (a *Anytype) HandlePeerFound(p peer.AddrInfo) {
	a.t.Host().Peerstore().AddAddrs(p.ID, p.Addrs, pstore.ConnectedAddrTTL)
}

func init() {
	net2.PullInterval = time.Minute * 3
	// apply log levels in go-threads and go-ipfs deps
	logging.ApplyLevelsFromEnv()
}

func NewFromOptions(options ...ServiceOption) (*Anytype, error) {
	opts := ServiceOptions{}
	opts.SetDefaults()

	for _, opt := range options {
		err := opt(&opts)
		if err != nil {
			return nil, err
		}
	}

	if opts.Device == nil {
		return nil, fmt.Errorf("no device keypair provided")
	}

	a := &Anytype{
		device:  opts.Device,
		account: opts.Account,

		repoPath:           opts.Repo,
		cafeGatewayBaseUrl: opts.WebGatewayBaseUrl,
		cafeP2PAddr:        opts.CafeP2PAddr,
		replicationWG:      sync.WaitGroup{},
		shutdownCh:         make(chan struct{}),
		onlineCh:           make(chan struct{}),
		smartBlockChanges:  broadcast.NewBroadcaster(0),
	}

	if opts.CafeGrpcHost != "" {
		var err error
		a.cafe, err = cafe.NewClient(opts.CafeGrpcHost, a.device, a.account)
		if err != nil {
			return nil, fmt.Errorf("failed to get grpc client: %w", err)
		}
	}

	if opts.NetBootstraper != nil {
		a.t = opts.NetBootstraper
	} else {
		var err error
		a.t, err = litenet.DefaultNetwork(
			opts.Repo,
			opts.Device,
			[]byte(ipfsPrivateNetworkKey),
			litenet.WithNetHostAddr(opts.HostAddr),
			litenet.WithNetDebug(false))
		if err != nil {
			return nil, err
		}
	}

	a.localStore = localstore.NewLocalStore(a.t.Datastore())
	a.files = files.New(a.localStore.Files, a.t.GetIpfs(), a.cafe)

	return a, nil
}

func New(rootPath string, account string) (Service, error) {
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

	a, err := NewFromOptions(WithRepo(repoPath), WithDeviceKey(deviceKp), WithAccountKey(accountKp), WithCafeGRPCHost(DefaultCafeNodeGRPC), WithHostMultiaddr(DefaultHostAddr))

	return a, err
}

func (a *Anytype) runPeriodicJobsInBackground() {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()

	go func() {
		for {
			select {
			case <-tick.C:
				//a.syncAccount(false)

			case <-a.shutdownCh:
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
	a.lock.Lock()
	defer a.lock.Unlock()

	go func() {
		a.t.Bootstrap(DefaultBoostrapPeers())
		// todo: init mdns discovery and register notifee
		// discovery.NewMdnsService
		close(a.onlineCh)
	}()

	return nil
}

func (a *Anytype) InitPredefinedBlocks(mustSyncFromRemote bool) error {
	err := a.createPredefinedBlocksIfNotExist(mustSyncFromRemote)
	if err != nil {
		return err
	}

	//a.runPeriodicJobsInBackground()
	return nil
}

func (a *Anytype) Stop() error {
	fmt.Printf("stopping the service %p\n", a.t)
	a.lock.Lock()
	defer a.lock.Unlock()

	if a.shutdownCh != nil {
		close(a.shutdownCh)
		a.shutdownCh = nil
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

	if a.db != nil {
		err := a.db.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
