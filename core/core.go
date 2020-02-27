package core

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/service"
	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-datastore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	pstore "github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-textile/keypair"
	"github.com/textileio/go-textile/strkey"
	"github.com/textileio/go-threads/store"
	"github.com/textileio/go-threads/util"
)

var log = logging.Logger("anytype-core")

const privateKey = `/key/swarm/psk/1.0.0/
/base16/
fee6e180af8fc354d321fde5c84cab22138f9c62fec0d1bc0e99f4439968b02c`

var BootstrapNodes = []string{
	"/ip4/68.183.2.167/tcp/4001/ipfs/12D3KooWB2Ya2GkLLRSR322Z13ZDZ9LP4fDJxauscYwUMKLFCqaD",
}

type PredefinedBlockIds struct {
	Profile string
	Home    string
	Archive string
}

type Anytype struct {
	ds       datastore.Batching
	repoPath string
	ts       store.ServiceBoostrapper
	mdns     discovery.Service
	account  *keypair.Full

	predefinedBlockIds PredefinedBlockIds
	logLevels          map[string]string
	lock               sync.Mutex
	done               chan struct{}
}

func (a *Anytype) Account() *keypair.Full {
	return a.account
}

func (a *Anytype) ipfs() *ipfslite.Peer {
	return a.ts.GetIpfsLite()
}

func (a *Anytype) IsStarted() bool {
	return a.ts != nil && a.ts.GetIpfsLite() != nil
}

// PredefinedBlockIds returns default blocks like home and archive
// ⚠️ Will return empty struct in case it runs before Anytype.Run()
func (a *Anytype) PredefinedBlockIds() PredefinedBlockIds {
	return a.predefinedBlockIds
}

func (a *Anytype) HandlePeerFound(p peer.AddrInfo) {
	a.ts.Host().Peerstore().AddAddrs(p.ID, p.Addrs, pstore.ConnectedAddrTTL)
}

func getLogLevels() map[string]string {
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

	return logLevels
}

func New(rootPath string, account string) (*Anytype, error) {
	repoPath := filepath.Join(rootPath, account)
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("not exists")
	}

	anytype := Anytype{repoPath: repoPath, logLevels: getLogLevels()}

	return &anytype, nil
}

func (a *Anytype) SetLogLevel(subsystem string, level string) {
	a.logLevels[subsystem] = strings.ToUpper(level)
}

func (a *Anytype) applyLogLevel() {
	if len(a.logLevels) == 0 {
		logging.SetAllLoggers(logging.LevelDebug)
		return
	}

	for subsystem, level := range a.logLevels {
		err := logging.SetLogLevel(subsystem, level)
		if err != nil {
			log.Fatalf("incorrect log level for %s: %s", subsystem, level)
		}
	}
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

func (a *Anytype) readKeyFile() (*keypair.Full, error) {
	pth := filepath.Join(a.repoPath, "key")
	_, err := os.Stat(pth)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("key file not exists")
	} else if err != nil {
		return nil, err
	}

	seed, err := ioutil.ReadFile(pth)
	if err != nil {
		return nil, err
	}

	if _, err = strkey.Decode(strkey.VersionByteSeed, string(seed)); err != nil {
		return nil, err
	}

	kp, err := keypair.Parse(string(seed))
	if err != nil {
		return nil, err
	}
	full, ok := kp.(*keypair.Full)
	if !ok {
		return nil, fmt.Errorf("invalid seed")
	}

	return full, nil
}

// Run start account
func (a *Anytype) Run() error {
	a.lock.Lock()
	defer a.lock.Unlock()
	hostAddr, err := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/4006")
	if err != nil {
		return err
	}

	kp, err := a.readKeyFile()
	if err != nil {
		return err
	}
	a.account = kp

	privKey, err := kp.LibP2PPrivKey()
	if err != nil {
		return err
	}

	ts, err := service.NewService(
		a.repoPath,
		privKey,
		[]byte(privateKey),
		service.WithServiceHostAddr(hostAddr),
		service.WithServiceDebug(true))
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
	ds, err := ipfslite.BadgerDatastore(filepath.Join(a.repoPath, "datastore"))
	if err != nil {
		return err
	}

	a.done = make(chan struct{})
	a.ts = ts
	a.ds = ds
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

	if a.ds != nil {
		err := a.ds.Close()
		if err != nil {
			return err
		}
	}

	return nil
}
