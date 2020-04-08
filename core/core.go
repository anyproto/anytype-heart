package core

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/cafeclient"
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

var (
	CafeNodeP2P  = "/dns4/cafe1.anytype.io/tcp/4001/p2p/12D3KooWKwPC165PptjnzYzGrEs7NSjsF5vvMmxmuqpA2VfaBbLw"
	CafeNodeGRPC = "/ip4/134.122.78.144/tcp/3006"
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
	repoPath           string
	t                  net.NetBoostrapper
	cafe               cafeclient.Client
	mdns               discovery.Service
	account            wallet.Keypair
	device             wallet.Keypair
	localStore         localstore.LocalStore
	predefinedBlockIds PredefinedBlockIds
	logLevels          map[string]string
	lock               sync.Mutex
	replicationWG      sync.WaitGroup
	done               chan struct{}
	online             bool
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
	CreateBlock(t SmartBlockType) (SmartBlock, error)

	FileAddWithBytes(ctx context.Context, content []byte, filename string) (File, error)
	FileAddWithReader(ctx context.Context, content io.Reader, filename string) (File, error)
	FileByHash(ctx context.Context, hash string) (File, error)

	ImageByHash(ctx context.Context, hash string) (Image, error)
	ImageAddWithBytes(ctx context.Context, content []byte, filename string) (Image, error)
	ImageAddWithReader(ctx context.Context, content io.Reader, filename string) (Image, error)
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
	// todo: rewrite with internal chan
	for {
		if a.online {
			ch <- nil
			close(ch)
			return
		}
		time.Sleep(time.Millisecond * 100)
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
	// apply log levels in go-threads and go-ipfs deps
	logging.ApplyLevelsFromEnv()
}

func New(rootPath string, account string) (Service, error) {
	repoPath := filepath.Join(rootPath, account)
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("not exists")
	}

	a := Anytype{repoPath: repoPath, replicationWG: sync.WaitGroup{}}
	var err error
	b, err := ioutil.ReadFile(filepath.Join(repoPath, keyFileAccount))
	if err != nil {
		return nil, fmt.Errorf("failed to read account keyfile: %w", err)
	}
	a.account, err = wallet.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}
	if a.account.KeypairType() != wallet.KeypairTypeAccount {
		return nil, fmt.Errorf("got %s key type instead of %s", a.account.KeypairType(), wallet.KeypairTypeAccount)
	}

	b, err = ioutil.ReadFile(filepath.Join(repoPath, keyFileDevice))
	if err != nil {
		return nil, fmt.Errorf("failed to read device keyfile: %w", err)
	}

	a.device, err = wallet.UnmarshalBinary(b)
	if err != nil {
		return nil, err
	}
	if a.device.KeypairType() != wallet.KeypairTypeDevice {
		return nil, fmt.Errorf("got %s key type instead of %s", a.device.KeypairType(), wallet.KeypairTypeDevice)
	}

	a.cafe, err = cafeclient.NewClient(CafeNodeGRPC, a.device, a.account)
	if err != nil {
		return nil, fmt.Errorf("failed to get grpc client: %w", err)
	}

	return &a, nil
}

func (a *Anytype) runPeriodicJobsInBackground() {
	tick := time.NewTicker(time.Hour)
	defer tick.Stop()

	go func() {
		for {
			select {
			case <-tick.C:
				//a.syncAccount(false)

			case <-a.done:
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
	hostAddr, err := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/4006")
	if err != nil {
		return err
	}

	ts, err := litenet.DefaultNetwork(
		a.repoPath,
		a.device,
		[]byte(ipfsPrivateNetworkKey),
		litenet.WithNetHostAddr(hostAddr),
		litenet.WithNetDebug(false))
	if err != nil {
		return err
	}

	go func() {
		ts.Bootstrap(DefaultBoostrapPeers())
		a.online = true
	}()

	// ctx := context.Background()
	/*mdns, err := discovery.NewMdnsService(ctx, t.Host(), time.Second, "")
	if err != nil {
		log.Fatal(err)
	}*/

	// todo: use the datastore from go-threads to save resources on the second instance
	//ds,= t.Datastore()

	a.done = make(chan struct{})
	a.t = ts

	a.localStore = localstore.NewLocalStore(a.t.Datastore())
	//	a.ds = ds
	//a.mdns = mdns
	//mdns.RegisterNotifee(a)

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
	fmt.Printf("stopping the node %p\n", a.t)
	a.lock.Lock()
	defer a.lock.Unlock()

	a.replicationWG.Wait()
	if a.done != nil {
		close(a.done)
		a.done = nil
	}

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

	/*if a.ds != nil {
		err := a.ds.Close()
		if err != nil {
			return err
		}
	}*/

	return nil
}
