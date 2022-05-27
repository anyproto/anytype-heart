package ipfslite

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-cid"
	ds "github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-ipns"
	uio "github.com/ipfs/go-unixfs/io"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/pnet"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	dualdht "github.com/libp2p/go-libp2p-kad-dht/dual"
	"github.com/libp2p/go-libp2p-peerstore/pstoreds"
	record "github.com/libp2p/go-libp2p-record"
	libp2ptls "github.com/libp2p/go-libp2p-tls"
	"github.com/libp2p/go-tcp-transport"
	websocket "github.com/libp2p/go-ws-transport"
	ma "github.com/multiformats/go-multiaddr"
	madns "github.com/multiformats/go-multiaddr-dns"
	"github.com/textileio/go-threads/util"

	app "github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/datastore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/net/resolver"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/util/nocloserds"
)

const CName = "ipfs"

var log = logging.Logger("anytype-core-litenet")

type liteNet struct {
	cfg *Config
	*ipfslite.Peer
	ds   datastore.Datastore
	host host.Host
	dht  *dualdht.DHT

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

	res, err := madns.NewResolver(
		madns.WithDefaultResolver(resolver.NewResolverWithTTL(time.Minute * 30)),
	)
	if err != nil {
		return err
	}
	madns.DefaultResolver = res

	ln.cfg, err = ln.getConfig(a)
	if err != nil {
		return err
	}

	return nil
}

func newDHT(ctx context.Context, h host.Host, ds ds.Batching) (*dualdht.DHT, error) {
	dhtOpts := []dualdht.Option{
		dualdht.DHTOption(dht.NamespacedValidator("pk", record.PublicKeyValidator{})),
		dualdht.DHTOption(dht.NamespacedValidator("ipns", ipns.Validator{KeyBook: h.Peerstore()})),
		dualdht.DHTOption(dht.Concurrency(10)),
		dualdht.DHTOption(dht.Mode(dht.ModeAuto)),
	}
	if ds != nil {
		dhtOpts = append(dhtOpts, dualdht.DHTOption(dht.Datastore(ds)))
	}

	return dualdht.New(ctx, h, dhtOpts...)
}

func setupLibP2PNode(ctx context.Context, cfg *Config, blockDS, peerDS ds.Batching) (host.Host, *dualdht.DHT, error) {
	var ddht *dualdht.DHT
	var err error

	pstore, err := pstoreds.NewPeerstore(ctx, peerDS, pstoreds.DefaultOpts())
	if err != nil {
		return nil, nil, err
	}

	r := bytes.NewReader([]byte(cfg.PrivateNetSecret))
	privateNetworkKey, err := pnet.DecodeV1PSK(r)
	if err != nil {
		return nil, nil, err
	}

	transports := libp2p.ChainOptions(
		libp2p.NoTransports,
		libp2p.Transport(tcp.NewTCPTransport, tcp.WithConnectionTimeout(time.Second*10)),
		libp2p.Transport(websocket.New),
	)

	cnmgr, err := connmgr.NewConnManager(cfg.SwarmLowWater, cfg.SwarmHighWater, connmgr.WithGracePeriod(time.Minute))
	if err != nil {
		return nil, nil, err
	}

	finalOpts := []libp2p.Option{
		libp2p.Identity(cfg.PrivKey),
		libp2p.ListenAddrs(cfg.HostAddr),
		libp2p.PrivateNetwork(privateNetworkKey),
		transports,
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			ddht, err = newDHT(ctx, h, blockDS)
			return ddht, err
		}),
		libp2p.ConnectionManager(cnmgr),
		libp2p.Peerstore(pstore),
		libp2p.Security(libp2ptls.ID, libp2ptls.New),
		libp2p.EnableAutoRelay(),            // if our network state changes we will try to connect to one of the relay specified below
		libp2p.StaticRelays(cfg.RelayNodes), // in case we are under NAT we will announce our addresses through these nodes
	}

	h, err := libp2p.New(
		finalOpts...,
	)
	if err != nil {
		return nil, nil, err
	}

	return h, ddht, err
}

func (ln *liteNet) Run() error {
	var ctx context.Context
	ctx, ln.peerStoreCtxCancel = context.WithCancel(context.Background())

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

	ln.host, ln.dht, err = setupLibP2PNode(ctx, ln.cfg, blockDS, peerDS)
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
	return i.Peer.HasBlock(context.Background(), c)
}

func (i *liteNet) Remove(ctx context.Context, c cid.Cid) error {
	return i.Peer.Remove(ctx, c)
}
