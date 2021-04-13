package ipfs

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	ipld "github.com/ipfs/go-ipld-format"
	uio "github.com/ipfs/go-unixfs/io"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"io"
)

const CName = "ipfs"

var log = logging.Logger("anytype-core-ipfs")

const (
	IpfsPrivateNetworkKey = `/key/swarm/psk/1.0.0/
/base16/
fee6e180af8fc354d321fde5c84cab22138f9c62fec0d1bc0e99f4439968b02c`
)

type AddParams struct {
	Layout    string
	Chunker   string
	RawLeaves bool
	Hidden    bool
	Shard     bool
	NoCopy    bool
	HashFun   string
}

type IPFS interface {
	ipld.DAGService

	Bootstrap(peers []peer.AddrInfo)
	Session(ctx context.Context) ipld.NodeGetter
	AddFile(ctx context.Context, r io.Reader, params *AddParams) (ipld.Node, error)
	GetFile(ctx context.Context, c cid.Cid) (uio.ReadSeekCloser, error)
	BlockStore() blockstore.Blockstore
	HasBlock(c cid.Cid) (bool, error)
}

type Node interface {
	app.ComponentRunnable
	IPFS
	GetHost() host.Host
	WaitBootstrap() (success bool) // waits until network bootstrap finished. Returns tru if at least 1 bootstrap node was connected successfully
}
