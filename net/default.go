package net

import (
	datastore "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	corenet "github.com/textileio/go-threads/core/net"

	"github.com/anytypeio/go-anytype-library/ipfs"
)

type NetBoostrapper interface {
	corenet.Net
	GetIpfs() ipfs.IPFS
	Bootstrap(addrs []peer.AddrInfo)
	Datastore() datastore.Batching
}
