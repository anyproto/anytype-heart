package ipfslite

import (
	"bytes"
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util/nocloserds"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	ipld "github.com/ipfs/go-ipld-format"
	uio "github.com/ipfs/go-unixfs/io"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/textileio/go-threads/util"
	"io"
	"time"

	ipfslite "github.com/hsanjuan/ipfs-lite"
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
	cfg *Config
	*ipfslite.Peer
	ds   datastore.Datastore
	host host.Host
	dht  *dual.DHT

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

	relayNodes, err := util.ParseBootstrapPeers(appCfg.RelayNodes)
	if err != nil {
		return nil, err
	}

	cfg := Config{
		HostAddr:         hostAddr,
		PrivKey:          keypair,
		PrivateNetSecret: appCfg.PrivateNetworkSecret,
		BootstrapNodes:   bootstrapNodes,
		RelayNodes:       relayNodes,
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
	ln.ds = a.MustComponent(datastore.CName).(datastore.Datastore)
	ln.bootstrapFinished = make(chan struct{})

	ln.cfg, err = ln.getConfig(a)
	if err != nil {
		return err
	}

	return nil
}

func (ln *liteNet) Run() error {
	peerDS, err := ln.ds.PeerstoreDS()
	if err != nil {
		return fmt.Errorf("peerDS: %s", err.Error())
	}
	blockDS, err := ln.ds.BlockstoreDS()
	if err != nil {
		return fmt.Errorf("blockDS: %s", err.Error())
	}

	peerDS = nocloserds.NewBatch(peerDS)
	blockDS = nocloserds.NewBatch(blockDS)

	var (
		ctx context.Context
	)

	ctx, ln.peerStoreCtxCancel = context.WithCancel(context.Background())
	pstore, err := pstoreds.NewPeerstore(ctx, peerDS, pstoreds.DefaultOpts())
	if err != nil {
		return err
	}

	r := bytes.NewReader([]byte(ln.cfg.PrivateNetSecret))
	privateNetworkKey, err := pnet.DecodeV1PSK(r)
	if err != nil {
		return err
	}

	ln.host, ln.dht, err = ipfslite.SetupLibp2p(
		ctx,
		ln.cfg.PrivKey,
		privateNetworkKey,
		[]ma.Multiaddr{ln.cfg.HostAddr},
		blockDS,
		libp2p.ConnectionManager(connmgr.NewConnManager(ln.cfg.SwarmLowWater, ln.cfg.SwarmHighWater, time.Minute)),
		libp2p.Peerstore(pstore),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.Transport(tcp.NewTCPTransport),  // connection timeout overridden in core.go init
		libp2p.EnableAutoRelay(),               // if our network state changes we will try to connect to one of the relay specified below
		libp2p.StaticRelays(ln.cfg.RelayNodes), // in case we are under NAT we will announce our addresses through these nodes
	)
	if err != nil {
		return err
	}

	ln.Peer, err = ipfslite.New(ctx, blockDS, ln.host, ln.dht, &ipfslite.Config{Offline: ln.cfg.Offline})
	if err != nil {
		return err
	}

	go func() {
		ln.Bootstrap(ln.cfg.BootstrapNodes)
		for _, p := range ln.cfg.BootstrapNodes {
			if ln.host.Network().Connectedness(p.ID) == network.Connected {
				ln.bootstrapSucceed = true
				break
			}
		}
		log.Infof("bootstrap finished. succeed = %v", ln.bootstrapSucceed)

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

func (ln *liteNet) GetHost() host.Host {
	return ln.host
}

func (ln *liteNet) Bootstrap(addrs []peer.AddrInfo) {
	// todo refactor: provide a way to check if bootstrap was finished or/and succesfull
	ln.Peer.Bootstrap(addrs)
}

func (ln *liteNet) Close() (err error) {
	if ln.peerStoreCtxCancel != nil {
		ln.peerStoreCtxCancel()
	}

	if ln.dht != nil {
		err = ln.dht.Close()
		if err != nil {
			return
		}
	}
	if ln.host != nil {
		err = ln.host.Close()
		if err != nil {
			return
		}
	}

	return nil
}

func (ln *liteNet) WaitBootstrapFinish() (success bool) {
	panic("implement me")
}

func (i *liteNet) Session(ctx context.Context) ipld.NodeGetter {
	return i.Peer.Session(ctx)
}

func (i *liteNet) AddFile(ctx context.Context, r io.Reader, params *ipfs.AddParams) (ipld.Node, error) {
	if params == nil {
		return i.Peer.AddFile(ctx, r, nil)
	}

	ipfsLiteParams := ipfslite.AddParams(*params)
	return i.Peer.AddFile(ctx, r, &ipfsLiteParams)
}

func (i *liteNet) GetFile(ctx context.Context, c cid.Cid) (uio.ReadSeekCloser, error) {
	return i.Peer.GetFile(ctx, c)
}

func (i *liteNet) BlockStore() blockstore.Blockstore {
	return i.Peer.BlockStore()
}

func (i *liteNet) HasBlock(c cid.Cid) (bool, error) {
	return i.Peer.HasBlock(c)
}
