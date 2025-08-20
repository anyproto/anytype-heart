package filesync

import (
	"context"
	"errors"
	"fmt"

	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"

	"github.com/anyproto/anytype-heart/core/domain"
)

const batchSize = 10

func (s *fileSync) walkFileBlocks(ctx context.Context, spaceId string, fileId domain.FileId, priorityBranches []domain.FileId, proc func(fileBlocks []blocks.Block) error) error {
	blocksBuf := make([]blocks.Block, 0, batchSize)

	err := s.walkDAG(ctx, spaceId, fileId, priorityBranches, func(node ipld.Node) error {
		b, err := blocks.NewBlockWithCid(node.RawData(), node.Cid())
		if err != nil {
			return err
		}
		blocksBuf = append(blocksBuf, b)
		if len(blocksBuf) == batchSize {
			err = proc(blocksBuf)
			if err != nil {
				return fmt.Errorf("process batch: %w", err)
			}
			blocksBuf = blocksBuf[:0]
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk DAG: %w", err)
	}

	if len(blocksBuf) > 0 {
		err = proc(blocksBuf)
		if err != nil {
			return fmt.Errorf("process batch: %w", err)
		}
	}
	return nil
}

func (s *fileSync) walkDAG(ctx context.Context, spaceId string, fileId domain.FileId, priorityBranches []domain.FileId, visit func(node ipld.Node) error) error {
	dagService := s.dagServiceForSpace(spaceId)

	visited := map[cid.Cid]struct{}{}

	priorityBranches = append(priorityBranches, fileId)
	for _, branchId := range priorityBranches {
		fileCid, err := cid.Parse(branchId.String())
		if err != nil {
			return fmt.Errorf("parse CID %s: %w", branchId, err)
		}
		rootNode, err := dagService.Get(ctx, fileCid)
		if err != nil {
			return fmt.Errorf("get root node: %w", err)
		}
		walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(rootNode, dagService))
		err = walker.Iterate(func(navNode ipld.NavigableNode) error {
			node := navNode.GetIPLDNode()
			if _, ok := visited[node.Cid()]; !ok {
				visited[node.Cid()] = struct{}{}
				return visit(node)
			}
			return nil
		})
		if errors.Is(err, ipld.EndOfDag) {
			err = nil
		}
		if err != nil {
			return err
		}
	}
	return nil
}
