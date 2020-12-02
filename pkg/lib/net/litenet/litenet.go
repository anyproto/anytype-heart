package litenet

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/sync"
	badger "github.com/ipfs/go-ds-badger"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/pnet"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-peerstore/pstoreds"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/textileio/go-threads/core/app"
	"github.com/textileio/go-threads/core/logstore"
	"github.com/textileio/go-threads/logstore/lstoreds"
	"github.com/textileio/go-threads/net"
	"google.golang.org/grpc"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs/ipfsliteinterface"
	tnet "github.com/anytypeio/go-anytype-middleware/pkg/lib/net"
)

var log = logging.Logger("anytype-core-litenet")

const (
	defaultIpfsLitePath = "ipfslite"
	defaultLogstorePath = "logstore"
)

func DefaultNetwork(
	repoPath string,
	privKey crypto.PrivKey,
	privateNetworkSecret []byte,
	opts ...NetOption,
) (tnet.NetBoostrapper, error) {
	var config NetConfig
	for _, opt := range opts {
		if err := opt(&config); err != nil {
			return nil, err
		}
	}

	if config.HostAddr == nil {
		addr, err := ma.NewMultiaddr("/ip4/0.0.0.0/tcp/0")
		if err != nil {
			return nil, err
		}
		config.HostAddr = addr
	}

	ipfsLitePath := filepath.Join(repoPath, defaultIpfsLitePath)
	if err := os.MkdirAll(ipfsLitePath, os.ModePerm); err != nil {
		return nil, err
	}
	var ds datastore.Batching
	if config.InMemoryDS {
		ds = sync.MutexWrap(datastore.NewMapDatastore())
	} else {
		var err error
		ds, err = badger.NewDatastore(ipfsLitePath, &badger.DefaultOptions)
		if err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	pstore, err := pstoreds.NewPeerstore(ctx, ds, pstoreds.DefaultOpts())
	if err != nil {
		ds.Close()
		cancel()
		return nil, err
	}
	r := bytes.NewReader(privateNetworkSecret)
	pnet, err := pnet.DecodeV1PSK(r)
	if err != nil {
		return nil, err
	}

	h, d, err := ipfslite.SetupLibp2p(
		ctx,
		privKey,
		pnet,
		[]ma.Multiaddr{config.HostAddr},
		ds,
		libp2p.ConnectionManager(connmgr.NewConnManager(100, 400, time.Minute)),
		libp2p.Peerstore(pstore),
	)

	if err != nil {
		cancel()
		ds.Close()
		return nil, err
	}

	lite, err := ipfslite.New(ctx, ds, h, d, &ipfslite.Config{Offline: config.Offline})
	if err != nil {
		cancel()
		ds.Close()
		return nil, err
	}
	// Build a logstore
	logstorePath := filepath.Join(repoPath, defaultLogstorePath)
	if err := os.MkdirAll(logstorePath, os.ModePerm); err != nil {
		return nil, err
	}
	logstore, err := badger.NewDatastore(logstorePath, &badger.DefaultOptions)
	if err != nil {
		cancel()
		ds.Close()
		return nil, err
	}
	tstore, err := lstoreds.NewLogstore(ctx, logstore, lstoreds.DefaultOpts())
	if err != nil {
		cancel()
		if err := logstore.Close(); err != nil {
			return nil, err
		}
		ds.Close()
		return nil, err
	}

	api, err := net.NewNetwork(ctx, h, lite.BlockStore(), lite, tstore, net.Config{
		Debug:        config.Debug,
		PubSub:       config.PubSub,
		SyncTracking: config.SyncTracking,
	}, config.GRPCServerOptions, config.GRPCDialOptions)
	if err != nil {
		cancel()
		if err := logstore.Close(); err != nil {
			return nil, err
		}
		ds.Close()
		return nil, err
	}

	return &netBoostrapper{
		cancel:            cancel,
		Net:               api,
		ipfs:              ipfsliteinterface.New(lite),
		pstore:            pstore,
		logstore:          tstore,
		logstoreDatastore: logstore,
		litestore:         ds,
		host:              h,
		lanDht:            d.LAN,
		wanDht:            d.WAN,
	}, nil
}

type NetConfig struct {
	HostAddr          ma.Multiaddr
	GRPCServerOptions []grpc.ServerOption
	GRPCDialOptions   []grpc.DialOption
	SyncTracking      bool
	Debug             bool
	Offline           bool
	InMemoryDS        bool // should be used for tests only

	PubSub bool
}

type NetOption func(c *NetConfig) error

func WithNetHostAddr(addr ma.Multiaddr) NetOption {
	return func(c *NetConfig) error {
		c.HostAddr = addr
		return nil
	}
}

func WithInMemoryDS(inMemoryDS bool) NetOption {
	return func(c *NetConfig) error {
		c.InMemoryDS = inMemoryDS
		return nil
	}
}

func WithOffline(offline bool) NetOption {
	return func(c *NetConfig) error {
		c.Offline = offline
		return nil
	}
}

func WithNetDebug(enabled bool) NetOption {
	return func(c *NetConfig) error {
		c.Debug = enabled
		return nil
	}
}

func WithNetPubSub(enabled bool) NetOption {
	return func(c *NetConfig) error {
		c.PubSub = enabled
		return nil
	}
}

func WithNetGRPCServerOptions(opts ...grpc.ServerOption) NetOption {
	return func(c *NetConfig) error {
		c.GRPCServerOptions = opts
		return nil
	}
}

func WithNetGRPCDialOptions(opts ...grpc.DialOption) NetOption {
	return func(c *NetConfig) error {
		c.GRPCDialOptions = opts
		return nil
	}
}

func WithNetSyncTracking() NetOption {
	return func(c *NetConfig) error {
		c.SyncTracking = true
		return nil
	}
}

type netBoostrapper struct {
	cancel context.CancelFunc
	app.Net
	ipfs              ipfs.IPFS
	pstore            peerstore.Peerstore
	logstoreDatastore datastore.Batching
	litestore         datastore.Batching
	logstore          logstore.Logstore

	host   host.Host
	wanDht *dht.IpfsDHT
	lanDht *dht.IpfsDHT
}

var _ tnet.NetBoostrapper = (*netBoostrapper)(nil)

func (tsb *netBoostrapper) Datastore() datastore.Batching {
	return tsb.logstoreDatastore
}

func (tsb *netBoostrapper) Logstore() logstore.Logstore {
	return tsb.logstore
}

func (tsb *netBoostrapper) Bootstrap(addrs []peer.AddrInfo) {
	tsb.ipfs.Bootstrap(addrs)
}

func (tsb *netBoostrapper) GetIpfs() ipfs.IPFS {
	return tsb.ipfs
}

func (tsb *netBoostrapper) Close() error {
	log.Debug("closing net...")
	if err := tsb.Net.Close(); err != nil {
		return err
	}
	tsb.cancel()
	log.Debug("closing lan dht...")
	if err := tsb.lanDht.Close(); err != nil {
		return err
	}
	log.Debug("closing wan dht...")
	if err := tsb.wanDht.Close(); err != nil {
		return err
	}
	log.Debug("closing libp2p host...")
	if err := tsb.host.Close(); err != nil {
		return err
	}
	log.Debug("closing pstore...")
	if err := tsb.pstore.Close(); err != nil {
		return err
	}
	log.Debug("closing litestore...")
	if err := tsb.litestore.Close(); err != nil {
		return err
	}
	log.Debug("closing logstore...")
	return tsb.logstoreDatastore.Close()
}

func LoadKey(repoPath string) (crypto.PrivKey, error) {
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
