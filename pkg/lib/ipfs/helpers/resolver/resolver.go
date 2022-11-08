// Package resolver implements utilities for resolving paths within ipfs.
package resolver

import (
	"context"
	"errors"
	"fmt"
	"time"

	path "github.com/ipfs/go-path"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	dag "github.com/ipfs/go-merkledag"
)

var log = logging.Logger("ipfs-pathresolv")

// ErrNoLink is returned when a link is not found in a path
type ErrNoLink struct {
	Name string
	Node cid.Cid
}

// Error implements the Error interface for ErrNoLink with a useful
// human readable message.
func (e ErrNoLink) Error() string {
	return fmt.Sprintf("no link named %q under %s", e.Name, e.Node.String())
}

// ResolveOnce resolves path through a single node
type ResolveOnce func(ctx context.Context, ds ipld.NodeGetter, nd ipld.Node, names []string) (*ipld.Link, []string, error)

// Resolver provides path resolution to IPFS
// It has a pointer to a DAGService, which is uses to resolve nodes.
// TODO: now that this is more modular, try to unify this code with the
//       the resolvers in namesys
type Resolver struct {
	DAG ipld.NodeGetter

	ResolveOnce ResolveOnce
}

// NewBasicResolver constructs a new basic resolver.
func NewBasicResolver(ds ipld.DAGService) *Resolver {
	return &Resolver{
		DAG:         ds,
		ResolveOnce: ResolveSingle,
	}
}

// ResolveToLastNode walks the given path and returns the cid of the last node
// referenced by the path
func (r *Resolver) ResolveToLastNode(ctx context.Context, fpath path.Path) (cid.Cid, []string, error) {
	c, p, err := path.SplitAbsPath(fpath)
	if err != nil {
		return cid.Cid{}, nil, err
	}

	if len(p) == 0 {
		return c, nil, nil
	}

	nd, err := r.DAG.Get(ctx, c)
	if err != nil {
		return cid.Cid{}, nil, err
	}

	for len(p) > 0 {
		lnk, rest, err := r.ResolveOnce(ctx, r.DAG, nd, p)

		// Note: have to drop the error here as `ResolveOnce` doesn't handle 'leaf'
		// paths (so e.g. for `echo '{"foo":123}' | ipfs dag put` we wouldn't be
		// able to resolve `zdpu[...]/foo`)
		if lnk == nil {
			break
		}

		if err != nil {
			if err == dag.ErrLinkNotFound {
				err = ErrNoLink{Name: p[0], Node: nd.Cid()}
			}
			return cid.Cid{}, nil, err
		}

		next, err := lnk.GetNode(ctx, r.DAG)
		if err != nil {
			return cid.Cid{}, nil, err
		}
		nd = next
		p = rest
	}

	if len(p) == 0 {
		return nd.Cid(), nil, nil
	}

	// Confirm the path exists within the object
	val, rest, err := nd.Resolve(p)
	if err != nil {
		if err == dag.ErrLinkNotFound {
			err = ErrNoLink{Name: p[0], Node: nd.Cid()}
		}
		return cid.Cid{}, nil, err
	}

	if len(rest) > 0 {
		return cid.Cid{}, nil, errors.New("path failed to resolve fully")
	}
	switch val.(type) {
	case *ipld.Link:
		return cid.Cid{}, nil, errors.New("inconsistent ResolveOnce / nd.Resolve")
	default:
		return nd.Cid(), p, nil
	}
}

// ResolveSingle simply resolves one hop of a path through a graph with no
// extra context (does not opaquely resolve through sharded nodes)
func ResolveSingle(ctx context.Context, ds ipld.NodeGetter, nd ipld.Node, names []string) (*ipld.Link, []string, error) {
	return nd.ResolveLink(names)
}

// ResolveLinks iteratively resolves names by walking the link hierarchy.
// Every node is fetched from the DAGService, resolving the next name.
// Returns the list of nodes forming the path, starting with ndd. This list is
// guaranteed never to be empty.
//
// ResolveLinks(nd, []string{"foo", "bar", "baz"})
// would retrieve "baz" in ("bar" in ("foo" in nd.Links).Links).Links
func (r *Resolver) ResolveLinks(ctx context.Context, ndd ipld.Node, names []string) ([]ipld.Node, error) {

	evt := log.EventBegin(ctx, "resolveLinks", logging.LoggableMap{"names": names})
	defer evt.Done()
	result := make([]ipld.Node, 0, len(names)+1)
	result = append(result, ndd)
	nd := ndd // dup arg workaround

	// for each of the path components
	for len(names) > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()

		lnk, rest, err := r.ResolveOnce(ctx, r.DAG, nd, names)
		if err == dag.ErrLinkNotFound {
			evt.Append(logging.LoggableMap{"error": err.Error()})
			return result, ErrNoLink{Name: names[0], Node: nd.Cid()}
		} else if err != nil {
			evt.Append(logging.LoggableMap{"error": err.Error()})
			return result, err
		}

		nextnode, err := lnk.GetNode(ctx, r.DAG)
		if err != nil {
			evt.Append(logging.LoggableMap{"error": err.Error()})
			return result, err
		}

		nd = nextnode
		result = append(result, nextnode)
		names = rest
	}
	return result, nil
}
