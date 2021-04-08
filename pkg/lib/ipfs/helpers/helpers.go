package helpers

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	swarm "github.com/libp2p/go-libp2p-swarm"
	ma "github.com/multiformats/go-multiaddr"
	"io"
	"io/ioutil"
	gopath "path"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/crypto/symmetric"
	"github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/go-merkledag"
	ipfspath "github.com/ipfs/go-path"
	"github.com/ipfs/go-path/resolver"
	uio "github.com/ipfs/go-unixfs/io"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/path"
	mh "github.com/multiformats/go-multihash"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var log = logging.Logger("anytype-ipfs")

// DataAtPath return bytes under an ipfs path
func DataAtPath(ctx context.Context, node ipfs.IPFS, pth string) (cid.Cid, symmetric.ReadSeekCloser, error) {
	resolvedPath, err := ResolvePath(ctx, node, path.New(pth))
	if err != nil {
		return cid.Undef, nil, fmt.Errorf("failed to resolve path %s: %w", pth, err)
	}

	r, err := node.GetFile(ctx, resolvedPath.Cid())
	if err != nil {
		return cid.Undef, nil, fmt.Errorf("failed to resolve path %s: %w", pth, err)
	}

	return resolvedPath.Cid(), r, nil
}

// DataAtCid return bytes under an ipfs path
func DataAtCid(ctx context.Context, node ipfs.IPFS, cid cid.Cid) ([]byte, error) {
	f, err := node.GetFile(ctx, cid)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var file files.File
	switch f := f.(type) {
	case files.File:
		file = f
	case files.Directory:
		return nil, iface.ErrIsDir
	default:
		return nil, iface.ErrNotSupported
	}

	return ioutil.ReadAll(file)
}

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

/*
func LsFiles(ctx context.Context, node *ipfslite.Peer, p path.Path) {
	ses := node.Session(ctx)
node.DAGService.

}
// LinksAtCid return ipld links under a path
func LinksAtCid(node *ipfslite.Peer, pth string) ([]*ipld.Link, error) {
	res, err := node.Ls(ctx, path.New(pth))
	if err != nil {
		return nil, err
	}

	links := make([]*ipld.Link, 0)
	for link := range res {
		links = append(links, &ipld.Link{
			Name: link.Name,
			Size: link.Size,
			Cid:  link.Cid,
		})
	}

	return links, nil
}*/

// AddDataToDirectory adds reader bytes to a virtual dir
func AddDataToDirectory(ctx context.Context, node ipfs.IPFS, dir uio.Directory, fname string, reader io.Reader) (*cid.Cid, error) {
	id, err := AddData(ctx, node, reader, false)
	if err != nil {
		return nil, err
	}

	n, err := node.Get(ctx, *id)
	if err != nil {
		return nil, err
	}

	err = dir.AddChild(ctx, fname, n)
	if err != nil {
		return nil, err
	}

	return id, nil
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

// AddData takes a reader and adds it, optionally pins it, optionally only hashes it
func AddData(ctx context.Context, node ipfs.IPFS, reader io.Reader, pin bool) (*cid.Cid, error) {
	pth, err := node.AddFile(ctx, files.NewReaderFile(reader), nil)
	log.Debugf("AddData: %s", pth.Cid().String())

	if err != nil {
		return nil, err
	}

	if pin {
		/*		err = api.Pin().Add(ctx, pth, options.Pin.Recursive(false))
				if err != nil {
					return nil, err
				}*/
	}
	id := pth.Cid()

	return &id, nil
}

// NodeAtLink returns the node behind an ipld link
func NodeAtLink(ctx context.Context, dag ipld.DAGService, link *ipld.Link) (ipld.Node, error) {
	return link.GetNode(ctx, dag)
}

// NodeAtCid returns the node behind a cid
func NodeAtCid(ctx context.Context, dag ipld.DAGService, id cid.Cid) (ipld.Node, error) {
	return dag.Get(ctx, id)
}

/*
// NodeAtPath returns the last node under path
func NodeAtPath(node *ipfslite.Peer, pth string, timeout time.Duration) (ipld.Node, error) {
	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(node.Context(), timeout)
	defer cancel()

	return api.ResolveNode(ctx, path.New(pth))
}*/

type Node struct {
	Links []Link
	Data  string
}

type Link struct {
	Name, Hash string
	Size       uint64
}

/*
// ObjectAtPath returns the DAG object at the given path
func ObjectAtPath(node *ipfslite.Peer, pth string) ([]byte, error) {
	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(node.Context(), CatTimeout)
	defer cancel()

	ipth := path.New(pth)
	nd, err := api.Object().Get(ctx, ipth)
	if err != nil {
		return nil, err
	}

	r, err := api.Object().Data(ctx, ipth)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	out := &Node{
		Links: make([]Link, len(nd.Links())),
		Data:  string(data),
	}

	for i, link := range nd.Links() {
		out.Links[i] = Link{
			Hash: link.Cid.String(),
			Name: link.Name,
			Size: link.Size,
		}
	}

	return json.Marshal(out)
}

// StatObjectAtPath returns info about an object
func StatObjectAtPath(node *ipfslite.Peer, pth string) (*iface.ObjectStat, error) {
	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(node.Context(), CatTimeout)
	defer cancel()

	return api.Object().Stat(ctx, path.New(pth))
}

// PinNode pins an ipld node
func PinNode(node *ipfslite.Peer, nd ipld.Node, recursive bool) error {
	ctx, cancel := context.WithTimeout(node.Context(), PinTimeout)
	defer cancel()

	defer node.Blockstore.PinLock().Unlock()

	err := node.Pinning.Pin(ctx, nd, recursive)
	if err != nil {
		if strings.Contains(err.Error(), "already pinned recursively") {
			return nil
		}
		return err
	}

	return node.Pinning.Flush()
}

// UnpinNode unpins an ipld node
func UnpinNode(node *ipfslite.Peer, nd ipld.Node, recursive bool) error {
	return UnpinCid(node, nd.Cid(), recursive)
}

// UnpinCid unpins a cid
func UnpinCid(node *ipfslite.Peer, id icid.Cid, recursive bool) error {
	ctx, cancel := context.WithTimeout(node.Context(), PinTimeout)
	defer cancel()

	err := node.Pinning.Unpin(ctx, id, recursive)
	if err != nil && err != pin.ErrNotPinned {
		return err
	}

	return node.Pinning.Flush()
}

// Pinned returns the subset of given cids that are pinned
func Pinned(node *ipfslite.Peer, cids []string) ([]icid.Cid, error) {
	var decoded []icid.Cid
	for _, id := range cids {
		dec, err := icid.Decode(id)
		if err != nil {
			return nil, err
		}
		decoded = append(decoded, dec)
	}
	list, err := node.Pinning.CheckIfPinned(decoded...)
	if err != nil {
		return nil, err
	}

	var pinned []icid.Cid
	for _, p := range list {
		if p.Mode != pin.NotPinned {
			pinned = append(pinned, p.Key)
		}
	}

	return pinned, nil
}

// NotPinned returns the subset of given cids that are not pinned
func NotPinned(node *ipfslite.Peer, cids []string) ([]icid.Cid, error) {
	var decoded []icid.Cid
	for _, id := range cids {
		dec, err := icid.Decode(id)
		if err != nil {
			return nil, err
		}
		decoded = append(decoded, dec)
	}
	list, err := node.Pinning.CheckIfPinned(decoded...)
	if err != nil {
		return nil, err
	}

	var notPinned []icid.Cid
	for _, p := range list {
		if p.Mode == pin.NotPinned {
			notPinned = append(notPinned, p.Key)
		}
	}

	return notPinned, nil
}
*/
// ResolveLinkByNames resolves a link in a node from a list of valid names
// Note: This exists for b/c w/ the "f" -> "meta" and "d" -> content migration
func ResolveLinkByNames(nd ipld.Node, names []string) (*ipld.Link, error) {
	for _, n := range names {
		link, _, err := nd.ResolveLink([]string{n})
		if err != nil {
			if err == merkledag.ErrLinkNotFound {
				continue
			}
			return nil, err
		}
		if link != nil {
			return link, nil
		}
	}
	return nil, nil
}

func PermanentConnection(ctx context.Context, addr ma.Multiaddr, host host.Host, retryInterval time.Duration) error {
	pidStr, _ := addr.ValueForProtocol(ma.P_P2P)
	pid, err := peer.Decode(pidStr)
	if err != nil {
		return fmt.Errorf("PermanentConnection invalid addr: %s", err.Error())
	}
	log.Errorf("PermanentConnection start %s", pid)

	go func() {
		for {
			state := host.Network().Connectedness(pid)
			// do not handle CanConnect purposefully
			if state == network.NotConnected || state == network.CannotConnect {
				if swrm, ok := host.Network().(*swarm.Swarm); ok {
					// clear backoff in order to connect more aggressively
					swrm.Backoff().Clear(pid)
				}
				err = host.Connect(ctx, peer.AddrInfo{
					ID:    pid,
					Addrs: []ma.Multiaddr{addr},
				})
				if err != nil {
					log.Warnf("PermanentConnection failed: %s", err.Error())
				} else {
					log.Debugf("PermanentConnection %s reconnected succesfully", pid.String())
				}
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(retryInterval):
				continue
			}
		}
	}()

	return nil
}
