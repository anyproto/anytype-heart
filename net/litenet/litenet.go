package litenet

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-datastore"
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
	corenet "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/logstore/lstoreds"
	"github.com/textileio/go-threads/net"
	"google.golang.org/grpc"

	"github.com/anytypeio/go-anytype-library/ipfs"
	"github.com/anytypeio/go-anytype-library/ipfs/ipfsliteinterface"
	net2 "github.com/anytypeio/go-anytype-library/net"
)

const (
	defaultIpfsLitePath = "ipfslite"
	defaultLogstorePath = "logstore"
)

func DefaultNetwork(repoPath string, privKey crypto.PrivKey, privateNetworkSecret []byte, opts ...NetOption) (net2.NetBoostrapper, error) {
	config := &NetConfig{}
	for _, opt := range opts {
		if err := opt(config); err != nil {
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
	litestore, err := badger.NewDatastore(ipfsLitePath, &badger.DefaultOptions)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	pstore, err := pstoreds.NewPeerstore(ctx, litestore, pstoreds.DefaultOpts())
	if err != nil {
		litestore.Close()
		cancel()
		return nil, err
	}
	r := bytes.NewReader(privateNetworkSecret)
	pnet, err := pnet.DecodeV1PSK(r)
	if err != nil {
		return nil, err
	}

	pnet = nil
	h, d, err := ipfslite.SetupLibp2p(
		ctx,
		privKey,
		pnet,
		[]ma.Multiaddr{config.HostAddr},
		litestore,
		libp2p.ConnectionManager(connmgr.NewConnManager(100, 400, time.Minute)),
		libp2p.Peerstore(pstore),
	)

	if err != nil {
		cancel()
		litestore.Close()
		return nil, err
	}

	lite, err := ipfslite.New(ctx, litestore, h, d, nil)
	if err != nil {
		cancel()
		litestore.Close()
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
		litestore.Close()
		return nil, err
	}
	tstore, err := lstoreds.NewLogstore(ctx, logstore, lstoreds.DefaultOpts())
	if err != nil {
		cancel()
		if err := logstore.Close(); err != nil {
			return nil, err
		}
		litestore.Close()
		return nil, err
	}

	api, err := net.NewNetwork(ctx, h, lite.BlockStore(), lite, tstore, net.Config{
		Debug: config.Debug,
	}, config.GRPCOptions...)
	if err != nil {
		cancel()
		if err := logstore.Close(); err != nil {
			return nil, err
		}
		litestore.Close()
		return nil, err
	}

	return &netBoostrapper{
		cancel:    cancel,
		Net:       api,
		ipfs:      ipfsliteinterface.New(lite),
		pstore:    pstore,
		logstore:  logstore,
		litestore: litestore,
		host:      h,
		dht:       d,
	}, nil
}

type NetConfig struct {
	HostAddr    ma.Multiaddr
	Debug       bool
	GRPCOptions []grpc.ServerOption
}

type NetOption func(c *NetConfig) error

func WithNetHostAddr(addr ma.Multiaddr) NetOption {
	return func(c *NetConfig) error {
		c.HostAddr = addr
		return nil
	}
}

func WithNetDebug(enabled bool) NetOption {
	return func(c *NetConfig) error {
		c.Debug = enabled
		return nil
	}
}

func WithNetGRPCOptions(opts ...grpc.ServerOption) NetOption {
	return func(c *NetConfig) error {
		c.GRPCOptions = opts
		return nil
	}
}

type netBoostrapper struct {
	cancel context.CancelFunc
	corenet.Net
	ipfs      ipfs.IPFS
	pstore    peerstore.Peerstore
	logstore  datastore.Batching
	litestore datastore.Batching
	host      host.Host
	dht       *dht.IpfsDHT
}

var _ net2.NetBoostrapper = (*netBoostrapper)(nil)

func (tsb *netBoostrapper) Datastore() datastore.Batching {
	return tsb.logstore
}

func (tsb *netBoostrapper) Bootstrap(addrs []peer.AddrInfo) {
	tsb.ipfs.Bootstrap(addrs)
}

func (tsb *netBoostrapper) GetIpfs() ipfs.IPFS {
	return tsb.ipfs
}

func (tsb *netBoostrapper) Close() error {
	if err := tsb.Net.Close(); err != nil {
		return err
	}
	tsb.cancel()
	if err := tsb.dht.Close(); err != nil {
		return err
	}
	if err := tsb.host.Close(); err != nil {
		return err
	}
	if err := tsb.pstore.Close(); err != nil {
		return err
	}
	if err := tsb.litestore.Close(); err != nil {
		return err
	}
	return tsb.logstore.Close()
	// Logstore closed by service
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
