package ipfslite

import (
	"github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

type Config struct {
	HostAddr         ma.Multiaddr
	Offline          bool
	PrivKey          crypto.PrivKey // takes precedence over PrivKeyFromPath
	PrivateNetSecret string
	BootstrapNodes   []peer.AddrInfo
	SwarmLowWater    int
	SwarmHighWater   int
}
