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
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-library/vclock"
	"github.com/anytypeio/go-anytype-library/wallet"
	"github.com/libp2p/go-libp2p-core/peer"
	pstore "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery"
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

	logLevels map[string]string

	opts              ServiceOptions
	smartBlockChanges *broadcast.Broadcaster

	replicationWG    sync.WaitGroup
	migrationOnce    sync.Once
	lock             sync.Mutex
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

	PageInfoWithLinks(id string) (*model.PageInfoWithLinks, error)
	PageList() ([]*model.PageInfo, error)
	PageUpdateLastOpened(id string) error
}

func (a *Anytype) Account() string {
	return a.opts.Account.Address()
}

func (a *Anytype) Device() string {
	return a.opts.Device.Address()
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
		case <-a.shutdownStartsCh:
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
	sb := &smartBlock{thread: thrd, node: a}
	err = sb.indexSnapshot(&smartBlockSnapshot{
		state:    vclock.New(),
		threadID: thrd.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to index new block %s: %s", thrd.ID.String(), err.Error())
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
		opts:          opts,
		replicationWG: sync.WaitGroup{},
		migrationOnce: sync.Once{},

		shutdownStartsCh:  make(chan struct{}),
		onlineCh:          make(chan struct{}),
		smartBlockChanges: broadcast.NewBroadcaster(0),
	}

	if opts.CafeGrpcHost != "" {
		var err error
		a.cafe, err = cafe.NewClient(opts.CafeGrpcHost, opts.Device, opts.Account)
		if err != nil {
			return nil, fmt.Errorf("failed to get grpc client: %w", err)
		}
	}

	return a, nil
}

func New(rootPath string, account string) (Service, error) {
	opts, err := getNewConfig(rootPath, account)
	if err != nil {
		return nil, err
	}

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

	return []ServiceOption{WithRepo(repoPath), WithDeviceKey(deviceKp), WithAccountKey(accountKp), WithCafeGRPCHost(DefaultCafeNodeGRPC), WithHostMultiaddr(DefaultHostAddr)}, nil
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

	if a.opts.NetBootstraper != nil {
		a.t = a.opts.NetBootstraper
	} else {
		var err error

		a.t, err = litenet.DefaultNetwork(
			a.opts.Repo,
			a.opts.Device,
			[]byte(ipfsPrivateNetworkKey),
			litenet.WithNetHostAddr(a.opts.HostAddr),
			litenet.WithNetDebug(false),
			litenet.WithOffline(a.opts.Offline))
		if err != nil {
			return err
		}
	}

	a.localStore = localstore.NewLocalStore(a.t.Datastore())
	a.files = files.New(a.localStore.Files, a.t.GetIpfs(), a.cafe)

	go func() {
		if a.opts.Offline {
			return
		}

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

	if a.db != nil {
		err := a.db.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
