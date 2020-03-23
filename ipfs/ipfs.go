package ipfs

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	ipfslite "github.com/hsanjuan/ipfs-lite"
	cid "github.com/ipfs/go-cid"
	icid "github.com/ipfs/go-cid"
	files "github.com/ipfs/go-ipfs-files"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	dag "github.com/ipfs/go-merkledag"
	ipfspath "github.com/ipfs/go-path"
	uio "github.com/ipfs/go-unixfs/io"
	iface "github.com/ipfs/interface-go-ipfs-core"
)

var log = logging.Logger("tex-ipfs")

const DefaultTimeout = time.Second * 5
const PinTimeout = time.Minute
const CatTimeout = time.Minute
const ConnectTimeout = time.Second * 10

// DataAtPath return bytes under an ipfs path
func DataAtPath(ctx context.Context, node *ipfslite.Peer, cid cid.Cid) ([]byte, error) {
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

func LinksAtPath(ctx context.Context, node *ipfslite.Peer, pth string) ([]*ipld.Link, error) {
	p, err := ipfspath.ParsePath(pth)
	if err != nil {
		return nil, err
	}

	_, pathCidStr, err := p.PopLastSegment()
	if err != nil {
		return nil, err
	}

	pathCid, err := cid.Parse(pathCidStr)
	if err != nil {
		return nil, err
	}

	dagNode, err := node.Get(ctx, pathCid)
	if err != nil {
		return nil, err
	}

	dir, err := uio.NewDirectoryFromNode(node, dagNode)
	if err != nil {
		return nil, err
	}

	return dir.Links(ctx)
}

/*
func LsFiles(ctx context.Context, node *ipfslite.Peer, p path.Path) {
	ses := node.Session(ctx)
node.DAGService.

}
// LinksAtPath return ipld links under a path
func LinksAtPath(node *ipfslite.Peer, pth string) ([]*ipld.Link, error) {
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
func AddDataToDirectory(ctx context.Context, node *ipfslite.Peer, dir uio.Directory, fname string, reader io.Reader) (*icid.Cid, error) {
	id, err := AddData(ctx, node, reader, false)
	if err != nil {
		return nil, err
	}

	fmt.Printf("AddDataToDirectory: %s %s\n", fname, id.String())

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
func AddLinkToDirectory(ctx context.Context, node *ipfslite.Peer, dir uio.Directory, fname string, pth string) error {
	fmt.Printf("AddLinkToDirectory: %s %s\n", fname, pth)
	id, err := icid.Decode(pth)
	if err != nil {
		return err
	}

	ctx2, cancel := context.WithTimeout(context.Background(), time.Second*20)
	defer cancel()

	nd, err := node.DAGService.Get(ctx2, id)
	if err != nil {
		return err
	}
	fmt.Printf("dag get ok: %s\n", id.String())

	return dir.AddChild(ctx, fname, nd)
}

// AddData takes a reader and adds it, optionally pins it, optionally only hashes it
func AddData(ctx context.Context, node *ipfslite.Peer, reader io.Reader, pin bool) (*icid.Cid, error) {
	pth, err := node.AddFile(ctx, files.NewReaderFile(reader), nil)
	fmt.Printf("AddData: %s\n", pth.Cid().String())

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
func NodeAtLink(ctx context.Context, node *ipfslite.Peer, link *ipld.Link) (ipld.Node, error) {
	return link.GetNode(ctx, node.DAGService)
}

// NodeAtCid returns the node behind a cid
func NodeAtCid(ctx context.Context, node *ipfslite.Peer, id icid.Cid) (ipld.Node, error) {
	return node.DAGService.Get(ctx, id)
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
			if err == dag.ErrLinkNotFound {
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
