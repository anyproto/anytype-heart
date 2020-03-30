package ipfs

import (
	"context"
	"io"

	cid "github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	ipld "github.com/ipfs/go-ipld-format"
	uio "github.com/ipfs/go-unixfs/io"
	"github.com/libp2p/go-libp2p-core/peer"
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
