package service

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	datastore "github.com/ipfs/go-datastore"
	badger "github.com/ipfs/go-ds-badger"
	libp2p "github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/crypto"
	host "github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	"github.com/libp2p/go-libp2p-peerstore/pstoreds"
	ma "github.com/multiformats/go-multiaddr"
	corenet "github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/logstore/lstoreds"
	"github.com/textileio/go-threads/net"
	"google.golang.org/grpc"
)

const (
	defaultIpfsLitePath = "ipfslite"
	defaultLogstorePath = "logstore"
)

// DefaultService is a boostrapable default Service with
// sane defaults.
type ServiceBoostrapper interface {
	corenet.Net
	GetIpfsLite() *ipfslite.Peer
	Bootstrap(addrs []peer.AddrInfo)
	Datastore() datastore.Batching
}

func NewService(repoPath string, privKey crypto.PrivKey, privateNetworkSecret []byte, opts ...ServiceOption) (ServiceBoostrapper, error) {
	config := &ServiceConfig{}
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
	litestore, err := ipfslite.BadgerDatastore(ipfsLitePath)
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
	h, d, err := ipfslite.SetupLibp2p(
		ctx,
		privKey,
		privateNetworkSecret,
		[]ma.Multiaddr{config.HostAddr},
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

	return &servBoostrapper{
		cancel:    cancel,
		Net:   api,
		litepeer:  lite,
		pstore:    pstore,
		logstore:  logstore,
		litestore: litestore,
		host:      h,
		dht:       d,
	}, nil
}

type ServiceConfig struct {
	HostAddr    ma.Multiaddr
	Debug       bool
	GRPCOptions []grpc.ServerOption
}

type ServiceOption func(c *ServiceConfig) error

func WithServiceHostAddr(addr ma.Multiaddr) ServiceOption {
	return func(c *ServiceConfig) error {
		c.HostAddr = addr
		return nil
	}
}

func WithServiceDebug(enabled bool) ServiceOption {
	return func(c *ServiceConfig) error {
		c.Debug = enabled
		return nil
	}
}

func WithServiceGRPCOptions(opts ...grpc.ServerOption) ServiceOption {
	return func(c *ServiceConfig) error {
		c.GRPCOptions = opts
		return nil
	}
}

type servBoostrapper struct {
	cancel context.CancelFunc
	corenet.Net
	litepeer  *ipfslite.Peer
	pstore    peerstore.Peerstore
	logstore  datastore.Datastore
	litestore datastore.Batching
	host      host.Host
	dht       *dht.IpfsDHT
}

var _ ServiceBoostrapper = (*servBoostrapper)(nil)

func (tsb *servBoostrapper) Datastore() datastore.Batching {
	return tsb.litestore
}

func (tsb *servBoostrapper) Identity() datastore.Batching {
	return tsb.litestore
}

func (tsb *servBoostrapper) Bootstrap(addrs []peer.AddrInfo) {
	tsb.litepeer.Bootstrap(addrs)
}

func (tsb *servBoostrapper) GetIpfsLite() *ipfslite.Peer {
	return tsb.litepeer
}

func (tsb *servBoostrapper) Close() error {
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
