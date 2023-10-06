package helpers

import (
	"context"
	"fmt"
	gopath "path"
	"time"

	"github.com/ipfs/boxo/coreiface/path"
	uio "github.com/ipfs/boxo/ipld/unixfs/io"
	ipfspath "github.com/ipfs/boxo/path"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	mh "github.com/multiformats/go-multihash"

	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers/resolver"
)

func LinksAtCid(ctx context.Context, dag ipld.DAGService, pathCid string) ([]*ipld.Link, error) {
	pathCidParsed, err := cid.Parse(pathCid)
	if err != nil {
		return nil, err
	}

	dagNode, err := dag.Get(ctx, pathCidParsed)
	if err != nil {
		return nil, err
	}

	dir, err := uio.NewDirectoryFromNode(dag, dagNode)
	if err != nil {
		return nil, err
	}

	dir.SetCidBuilder(cid.V1Builder{Codec: cid.DagProtobuf, MhType: mh.SHA2_256})

	return dir.Links(ctx)
}

func ResolvePath(ctx context.Context, dag ipld.DAGService, p path.Path) (path.Resolved, error) {
	if _, ok := p.(path.Resolved); ok {
		return p.(path.Resolved), nil
	}
	if err := p.IsValid(); err != nil {
		return nil, err
	}

	ipath := ipfspath.Path(p.String())

	var resolveOnce resolver.ResolveOnce
	switch ipath.Segments()[0] {
	case "ipfs":
		resolveOnce = uio.ResolveUnixfsOnce
	case "ipld":
		resolveOnce = resolver.ResolveSingle
	default:
		return nil, fmt.Errorf("unsupported path namespace: %s", p.Namespace())
	}
	r := &resolver.Resolver{
		DAG:         dag,
		ResolveOnce: resolveOnce,
	}

	node, rest, err := r.ResolveToLastNode(ctx, ipath)
	if err != nil {
		return nil, err
	}

	root, err := cid.Parse(ipath.Segments()[1])
	if err != nil {
		return nil, err
	}

	return path.NewResolvedPath(ipath, node, root, gopath.Join(rest...)), nil
}

// AddLinkToDirectory adds a link to a virtual dir
func AddLinkToDirectory(ctx context.Context, dag ipld.DAGService, dir uio.Directory, fname string, pth string) error {
	id, err := cid.Decode(pth)
	if err != nil {
		return err
	}

	ctx2, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	nd, err := dag.Get(ctx2, id)
	if err != nil {
		return err
	}

	return dir.AddChild(ctx, fname, nd)
}

// NodeAtLink returns the node behind an ipld link
func NodeAtLink(ctx context.Context, dag ipld.DAGService, link *ipld.Link) (ipld.Node, error) {
	return link.GetNode(ctx, dag)
}

type Node struct {
	Links []Link
	Data  string
}

type Link struct {
	Name, Hash string
	Size       uint64
}
