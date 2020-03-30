package ipfsliteinterface

import (
	"context"
	"io"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	"github.com/ipfs/go-cid"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	ipld "github.com/ipfs/go-ipld-format"
	uio "github.com/ipfs/go-unixfs/io"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/anytypeio/go-anytype-library/ipfs"
)

var _ ipfs.IPFS = (*ipfsLite)(nil)

type ipfsLite struct {
	ipld.DAGService
	l *ipfslite.Peer
}

func (i ipfsLite) Bootstrap(peers []peer.AddrInfo) {
	i.l.Bootstrap(peers)
}

func (i ipfsLite) Session(ctx context.Context) ipld.NodeGetter {
	return i.l.Session(ctx)
}

func (i ipfsLite) AddFile(ctx context.Context, r io.Reader, params *ipfs.AddParams) (ipld.Node, error) {
	if params == nil {
		return i.l.AddFile(ctx, r, nil)
	}

	ipfsLiteParams := ipfslite.AddParams(*params)
	return i.l.AddFile(ctx, r, &ipfsLiteParams)
}

func (i ipfsLite) GetFile(ctx context.Context, c cid.Cid) (uio.ReadSeekCloser, error) {
	return i.l.GetFile(ctx, c)
}

func (i ipfsLite) BlockStore() blockstore.Blockstore {
	return i.l.BlockStore()
}

func (i ipfsLite) HasBlock(c cid.Cid) (bool, error) {
	return i.HasBlock(c)
}

func New(p *ipfslite.Peer) ipfs.IPFS {
	return &ipfsLite{
		DAGService: p.DAGService,
		l:          p,
	}
}
