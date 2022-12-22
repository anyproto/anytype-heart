package helpers

import (
	"context"
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/metrics"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs/helpers/resolver"
	"github.com/ipfs/go-cid"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/swarm"
	ma "github.com/multiformats/go-multiaddr"
	"net"
	gopath "path"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/crypto/symmetric"
	ipld "github.com/ipfs/go-ipld-format"
	ipfspath "github.com/ipfs/go-path"
	uio "github.com/ipfs/go-unixfs/io"
	"github.com/ipfs/interface-go-ipfs-core/path"
	mh "github.com/multiformats/go-multihash"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/ipfs"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var log = logging.Logger("anytype-ipfs")

const (
	netTcpHealthCheckAddress = "healthcheck.anytype.io:80"
	netTcpHealthCheckTimeout = time.Second * 3
)

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

func PermanentConnection(ctx context.Context, addr ma.Multiaddr, host host.Host, retryInterval time.Duration, grpcConnected func() bool) error {
	addrInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return fmt.Errorf("PermanentConnection invalid addr: %s", err.Error())
	}

	var (
		state       network.Connectedness
		stateChange time.Time
	)
	go func() {
		d := net.Dialer{Timeout: netTcpHealthCheckTimeout}
		for {
			state2 := host.Network().Connectedness(addrInfo.ID)
			// do not handle CanConnect purposefully
			if state2 == network.NotConnected || state2 == network.CannotConnect {
				if swrm, ok := host.Network().(*swarm.Swarm); ok {
					// clear backoff in order to connect more aggressively
					swrm.Backoff().Clear(addrInfo.ID)
				}

				err = host.Connect(ctx, *addrInfo)
				state2 = host.Network().Connectedness(addrInfo.ID)
				if err != nil {
					log.Warnf("PermanentConnection failed: %s", err.Error())
				} else {
					log.Debugf("PermanentConnection %s reconnected succesfully", addrInfo.ID.String())
				}
			}

			if state2 != state || stateChange.IsZero() {
				if stateChange.IsZero() {
					// first iteration
					stateChange = time.Now()
				}

				event := metrics.CafeP2PConnectStateChanged{
					AfterMs:       time.Since(stateChange).Milliseconds(),
					PrevState:     int(state),
					NewState:      int(state2),
					GrpcConnected: grpcConnected(),
				}
				if state2 != network.Connected {
					c, err := d.Dial("tcp", netTcpHealthCheckAddress)
					if err == nil {
						_ = c.Close()
					} else {
						event.NetCheckError = err.Error()
					}

					event.NetCheckSuccess = err == nil
				}

				stateChange = time.Now()
				state = state2
				metrics.SharedClient.RecordEvent(event)
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
