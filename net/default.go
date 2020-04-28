package net

import (
	"github.com/anytypeio/go-anytype-library/ipfs"
	datastore "github.com/ipfs/go-datastore"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/textileio/go-threads/core/app"
	"github.com/textileio/go-threads/core/logstore"
)

type NetBoostrapper interface {
	app.Net
	GetIpfs() ipfs.IPFS
	Bootstrap(addrs []peer.AddrInfo)
	Datastore() datastore.Batching
	Logstore() logstore.Logstore
	DatastoreWasInited() bool // true when the datastore wasn't existing in the FS before (just created)
}
