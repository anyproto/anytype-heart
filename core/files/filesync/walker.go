package filesync

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/ipld/unixfs"
	"github.com/ipfs/boxo/ipld/unixfs/importer/helpers"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
)

var errBlockNotFound = errors.New("block not found")

// leafSizeSlack covers the dag-pb and unixfs envelope around a leaf chunk when
// comparing a link's cumulative size against the chunk size
const leafSizeSlack = 4096

// maxSingleLayerFileSize is the largest file content the balanced unixfs
// importer lays out as a single layer of leaf chunks under one file node.
// Larger files get intermediate nodes, so children of their top node are not
// leaves.
var maxSingleLayerFileSize = uint64(helpers.DefaultLinksPerBlock) * uint64(fileservice.ChunkSize)

type dagEntry struct {
	cid  cid.Cid
	size int
}

// enumerateFileCids collects every cid of a file DAG together with its block
// size, without requiring the file's data to be present locally.
//
// Structural nodes (directories, file nodes with links) are read from the
// local store first, with a fallback to the file node (and cached locally).
// Leaf chunks are never fetched: when a node is provably a single layer of
// leaf chunks, its children are enumerated from the node's links (cid +
// size). A fully-remote file thus costs a few KB of structural nodes to
// enumerate, not the whole file.
//
// Returns errBlockNotFound (wrapped) when a node to fetch exists neither
// locally nor on the file node.
//
// Branches are walked in the given order, so that entries of small file
// variants come first; the file root is walked last and covers everything not
// visited yet.
func (s *fileSync) enumerateFileCids(ctx context.Context, spaceId string, fileId domain.FileId, priorityBranches []domain.FileId) ([]dagEntry, error) {
	type stackItem struct {
		c    cid.Cid
		size uint64
		leaf bool
	}

	branchIds := make([]domain.FileId, 0, len(priorityBranches)+1)
	branchIds = append(branchIds, priorityBranches...)
	branchIds = append(branchIds, fileId)

	branches := make([]cid.Cid, 0, len(branchIds))
	for _, branchId := range branchIds {
		branchCid, err := cid.Parse(branchId.String())
		if err != nil {
			return nil, fmt.Errorf("parse CID %s: %w", branchId, err)
		}
		branches = append(branches, branchCid)
	}

	visited := map[cid.Cid]struct{}{}
	var entries []dagEntry
	for _, branch := range branches {
		stack := []stackItem{{c: branch}}
		for len(stack) > 0 {
			item := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			if _, ok := visited[item.c]; ok {
				continue
			}
			visited[item.c] = struct{}{}

			// A raw block can't have children; its size is known from the
			// parent's link, no need to fetch it
			if item.leaf || (item.c.Prefix().Codec == cid.Raw && item.size > 0) {
				entries = append(entries, dagEntry{cid: item.c, size: int(item.size)})
				continue
			}

			node, err := s.getNode(ctx, spaceId, item.c)
			if err != nil {
				return nil, fmt.Errorf("get node %s: %w", item.c, err)
			}
			entries = append(entries, dagEntry{cid: item.c, size: len(node.RawData())})

			links := node.Links()
			if len(links) == 0 {
				continue
			}
			leafChildren := childrenAreLeafChunks(node, links)
			// Push in reverse so children pop in link order
			for i := len(links) - 1; i >= 0; i-- {
				link := links[i]
				stack = append(stack, stackItem{
					c:    link.Cid,
					size: link.Size,
					leaf: leafChildren && link.Size <= uint64(fileservice.ChunkSize)+leafSizeSlack,
				})
			}
		}
	}
	return entries, nil
}

// childrenAreLeafChunks reports whether every child of the node is provably a
// leaf chunk of file content, so that children can be enumerated from links
// without fetching them.
//
// This holds for a unixfs file node whose content fits in a single layer of
// chunks: the balanced importer then puts all leaves directly under it. The
// check is exact for DAGs built with our chunk size (fileservice.ChunkSize).
// A DAG built with a smaller chunker can, when its chunk count is ≡1 modulo
// the link fanout, hide a one-chunk intermediate node behind a small link and
// have it misread as a leaf; the consequence is one unenumerated block, never
// a wrong upload. When in doubt this function returns false and the children
// are fetched, which is always correct.
func childrenAreLeafChunks(node ipld.Node, links []*ipld.Link) bool {
	protoNode, ok := node.(*merkledag.ProtoNode)
	if !ok {
		return false
	}
	fsNode, err := unixfs.FSNodeFromBytes(protoNode.Data())
	if err != nil {
		return false
	}
	if fsNode.Type() != unixfs.TFile {
		return false
	}
	if fsNode.NumChildren() != len(links) {
		return false
	}
	if fsNode.FileSize() > maxSingleLayerFileSize {
		return false
	}
	for i := range links {
		if fsNode.BlockSize(i) > uint64(fileservice.ChunkSize) {
			return false
		}
	}
	return true
}

// getNode reads a DAG node from the local store, falling back to the file
// node. Remotely fetched blocks are cached locally, so repeated enumerations
// of the same file hit the local store.
func (s *fileSync) getNode(ctx context.Context, spaceId string, c cid.Cid) (ipld.Node, error) {
	ctx = fileblockstore.CtxWithSpaceId(ctx, spaceId)
	node, err := s.dagServiceForSpace(spaceId).Get(ctx, c)
	if err == nil {
		return node, nil
	}
	if !isBlockNotFoundError(err) {
		return nil, fmt.Errorf("get local node: %w", err)
	}

	b, err := s.rpcStore.Get(ctx, c)
	if err != nil {
		if isBlockNotFoundError(err) {
			return nil, errors.Join(err, errBlockNotFound)
		}
		return nil, fmt.Errorf("get remote node: %w", err)
	}
	node, err = decodeBlock(b)
	if err != nil {
		return nil, fmt.Errorf("decode remote node: %w", err)
	}
	if addErr := s.fileStorage.Add(ctx, []blocks.Block{b}); addErr != nil {
		log.Warn("cache remotely fetched node locally", zap.String("cid", c.String()), zap.Error(addErr))
	}
	return node, nil
}

func decodeBlock(b blocks.Block) (ipld.Node, error) {
	switch b.Cid().Prefix().Codec {
	case cid.DagProtobuf:
		return merkledag.DecodeProtobufBlock(b)
	case cid.Raw:
		return merkledag.DecodeRawBlock(b)
	default:
		return nil, fmt.Errorf("unsupported codec %d", b.Cid().Prefix().Codec)
	}
}

func isBlockNotFoundError(err error) bool {
	return ipld.IsNotFound(err) ||
		errors.Is(err, fileprotoerr.ErrCIDNotFound) ||
		strings.Contains(err.Error(), "failed to fetch all nodes")
}
