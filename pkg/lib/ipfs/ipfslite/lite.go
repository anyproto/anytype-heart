package ipfslite

import (
	"bytes"
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	datastore2 "github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs/ipfslite/ipfsliteinterface"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util/nocloserds"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/textileio/go-threads/util"
	"io/ioutil"
	"os"
	"time"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/pnet"
	"github.com/libp2p/go-libp2p-peerstore/pstoreds"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	"github.com/libp2p/go-tcp-transport"
	ma "github.com/multiformats/go-multiaddr"

	app "github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

const CName = "ipfs"

var log = logging.Logger("anytype-core-litenet")

type liteNet struct {
	cfg                *Config
	peerDS             datastore.Batching
	blockDS            datastore.Batching
	host               host.Host
	dht                *dual.DHT
	lite               *ipfslite.Peer
	peerStoreCtxCancel context.CancelFunc

	bootstrapSucceed  bool
	bootstrapFinished chan struct{}
}

func New() ipfs.Node {
	return &liteNet{}
}

func (ln *liteNet) getConfig(a *app.App) (*Config, error) {
	appCfg := a.MustComponent(config.CName).(*config.Config)
	wl := a.MustComponent(wallet.CName).(wallet.Wallet)

	keypair, err := wl.GetDevicePrivkey()
	if err != nil {
		return nil, fmt.Errorf("failed to get device keypair: %v", err)
	}

	hostAddrStr := appCfg.HostAddr
	if hostAddrStr == "" {
		hostAddrStr = "/ip4/0.0.0.0/tcp/0"
	}
	hostAddr, err := ma.NewMultiaddr(hostAddrStr)
	if err != nil {
		return nil, err
	}

	bootstrapNodes, err := util.ParseBootstrapPeers(appCfg.BootstrapNodes)
	if err != nil {
		return nil, err
	}
	cfg := Config{
		HostAddr:         hostAddr,
		PrivKey:          keypair,
		PrivateNetSecret: appCfg.PrivateNetworkSecret,
		BootstrapNodes:   bootstrapNodes,
		SwarmLowWater:    appCfg.SwarmLowWater,
		SwarmHighWater:   appCfg.SwarmHighWater,
		Offline:          appCfg.Offline,
	}

	if cfg.PrivateNetSecret == "" {
		// todo: remove this temporarily error in order to be able to connect to public IPFS
		return nil, fmt.Errorf("private network secret is nil")
	}

	return &cfg, nil
}

func (ln *liteNet) Init(a *app.App) (err error) {
	ds := a.MustComponent(datastore2.CName).(datastore2.Datastore)
	if ds == nil {
		return fmt.Errorf("ds is nil")
	}

	ln.bootstrapFinished = make(chan struct{})
	ln.peerDS = ds.PeerstoreDS()
	ln.blockDS = ds.BlockstoreDS()

	if ln.peerDS == nil {
		return fmt.Errorf("peerDS is nil")
	}
	if ln.blockDS == nil {
		return fmt.Errorf("blockDS is nil")
	}

	ln.cfg, err = ln.getConfig(a)
	if err != nil {
		return err
	}

	peerDS := nocloserds.NewBatch(ln.peerDS)
	blockDS := nocloserds.NewBatch(ln.blockDS)

	var (
		ctx context.Context
	)

	ctx, ln.peerStoreCtxCancel = context.WithCancel(context.Background())
	pstore, err := pstoreds.NewPeerstore(ctx, peerDS, pstoreds.DefaultOpts())
	if err != nil {
		return err
	}

	r := bytes.NewReader([]byte(ln.cfg.PrivateNetSecret))
	pnet, err := pnet.DecodeV1PSK(r)
	if err != nil {
		return err
	}

	ln.host, ln.dht, err = ipfslite.SetupLibp2p(
		ctx,
		ln.cfg.PrivKey,
		pnet,
		[]ma.Multiaddr{ln.cfg.HostAddr},
		blockDS,
		libp2p.ConnectionManager(connmgr.NewConnManager(ln.cfg.SwarmLowWater, ln.cfg.SwarmHighWater, time.Minute)),
		libp2p.Peerstore(pstore),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Transport(tcp.NewTCPTransport), // connection timeout overridden in core.go init
	)
	if err != nil {
		return err
	}

	ln.lite, err = ipfslite.New(ctx, blockDS, ln.host, ln.dht, &ipfslite.Config{Offline: ln.cfg.Offline})
	if err != nil {
		return err
	}

	return nil
}

func (ln *liteNet) Run() error {
	go func() {
		log.Errorf("bootstrap started")
		ln.Bootstrap(ln.cfg.BootstrapNodes)
		for _, p := range ln.cfg.BootstrapNodes {
			if ln.host.Network().Connectedness(p.ID) == network.Connected {
				ln.bootstrapSucceed = true
				break
			}
		}
		log.Errorf("bootstrap finished. succeed = %v", ln.bootstrapSucceed)

		close(ln.bootstrapFinished)
	}()
	return nil
}

func (ln *liteNet) Name() (name string) {
	return CName
}

func (ln *liteNet) WaitBootstrap() bool {
	<-ln.bootstrapFinished
	return ln.bootstrapSucceed
}

func (ln *liteNet) GetIpfs() ipfs.IPFS {
	return ipfsliteinterface.New(ln.lite)
}

func (ln *liteNet) GetHost() host.Host {
	return ln.host
}

func (ln *liteNet) Bootstrap(addrs []peer.AddrInfo) {
	// todo refactor: provide a way to check if bootstrap was finished or/and succesfull
	ln.lite.Bootstrap(addrs)
}

func (ln *liteNet) Close() (err error) {
	if ln.peerStoreCtxCancel != nil {
		ln.peerStoreCtxCancel()
	}

	err = ln.dht.Close()
	if err != nil {
		return
	}
	err = ln.host.Close()
	if err != nil {
		return
	}

	return nil
}

func loadKey(repoPath string) (crypto.PrivKey, error) {
	_, err := os.Stat(repoPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("key file not exists")
	} else if err != nil {
		return nil, err
	}

	var priv crypto.PrivKey

	key, err := ioutil.ReadFile(repoPath)
	if err != nil {
		panic(err)
	}
	priv, err = crypto.UnmarshalPrivateKey(key)
	if err != nil {
		panic(err)
	}

	return priv, nil
}

func (ln *liteNet) WaitBootstrapFinish() (success bool) {
	panic("implement me")
}
