package core

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	pstore "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-threads/util"

	"github.com/anytypeio/go-anytype-library/localstore"
	"github.com/anytypeio/go-anytype-library/service"
	"github.com/anytypeio/go-anytype-library/wallet"
)

var log = logging.Logger("anytype-core")

const (
	ipfsPrivateNetworkKey = `/key/swarm/psk/1.0.0/
/base16/
fee6e180af8fc354d321fde5c84cab22138f9c62fec0d1bc0e99f4439968b02c`

	keyFileAccount = "account.key"
	keyFileDevice  = "device.key"
)

var BootstrapNodes = []string{
	"/ip4/68.183.2.167/tcp/4001/ipfs/12D3KooWB2Ya2GkLLRSR322Z13ZDZ9LP4fDJxauscYwUMKLFCqaD",
}

type PredefinedBlockIds struct {
	Account string
	Profile string
	Home    string
	Archive string
}

type Anytype struct {
	repoPath           string
	ts                 service.NetBoostrapper
	mdns               discovery.Service
	account            wallet.Keypair
	device             wallet.Keypair
	localStore         localstore.LocalStore
	predefinedBlockIds PredefinedBlockIds
	logLevels          map[string]string
	lock               sync.Mutex
	done               chan struct{}
}

type Service interface {
	Account() string
	Start() error
	Stop() error
	IsStarted() bool

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

func (a *Anytype) ipfs() *ipfslite.Peer {
	return a.ts.GetIpfsLite()
}

func (a *Anytype) IsStarted() bool {
	return a.ts != nil && a.ts.GetIpfsLite() != nil
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
	a.ts.Host().Peerstore().AddAddrs(p.ID, p.Addrs, pstore.ConnectedAddrTTL)
}

func applyLogLevels() {
	levels := os.Getenv("ANYTYPE_LOG_LEVEL")
	logLevels := make(map[string]string)
	if levels != "" {
		for _, level := range strings.Split(levels, ";") {
			parts := strings.Split(level, "=")
			if len(parts) == 1 {
				for _, subsystem := range logging.GetSubsystems() {
					if strings.HasPrefix(subsystem, "anytype-") {
						logLevels[subsystem] = parts[0]
					}
				}
			} else if len(parts) == 2 {
				logLevels[parts[0]] = parts[1]
			}
		}
	}

	if len(logLevels) == 0 {
		logging.SetAllLoggers(logging.LevelDebug)
		return
	}

	for subsystem, level := range logLevels {
		err := logging.SetLogLevel(subsystem, level)
		if err != nil {
			log.Fatalf("incorrect log level for %s: %s", subsystem, level)
		}
	}
}

func init() {
	applyLogLevels()
}

func New(rootPath string, account string) (Service, error) {
	repoPath := filepath.Join(rootPath, account)
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("not exists")
	}

	a := Anytype{repoPath: repoPath}
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

func (a *Anytype) Start() error {
	a.lock.Lock()
	defer a.lock.Unlock()
	hostAddr, err := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/4006")
	if err != nil {
		return err
	}

	ts, err := service.DefaultNetwork(
		a.repoPath,
		a.device,
		[]byte(ipfsPrivateNetworkKey),
		service.WithNetHostAddr(hostAddr),
		service.WithNetDebug(true))
	if err != nil {
		return err
	}

	ts.Bootstrap(util.DefaultBoostrapPeers())

	ctx := context.Background()
	mdns, err := discovery.NewMdnsService(ctx, ts.Host(), time.Second, "")
	if err != nil {
		log.Fatal(err)
	}

	// todo: use the datastore from go-threads to save resources on the second instance
	//ds,= ts.Datastore()

	a.done = make(chan struct{})
	a.ts = ts
	a.localStore = localstore.NewLocalStore(a.ts.Datastore())
	//	a.ds = ds
	a.mdns = mdns
	mdns.RegisterNotifee(a)

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
	fmt.Printf("stopping the node %p\n", a.ts)
	a.lock.Lock()
	defer a.lock.Unlock()

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

	if a.ts != nil {
		err := a.ts.Close()
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
