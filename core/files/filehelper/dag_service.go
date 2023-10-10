package filehelper

import (
	"context"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

type DAGServiceWithSpaceID struct {
	spaceID    string
	dagService ipld.DAGService
}

func NewDAGServiceWithSpaceID(spaceID string, dagService ipld.DAGService) ipld.DAGService {
	return &DAGServiceWithSpaceID{
		spaceID:    spaceID,
		dagService: dagService,
	}
}

func (s *DAGServiceWithSpaceID) Get(ctx context.Context, cid cid.Cid) (ipld.Node, error) {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.Get(ctx, cid)
}

func (s *DAGServiceWithSpaceID) GetMany(ctx context.Context, cids []cid.Cid) <-chan *ipld.NodeOption {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.GetMany(ctx, cids)
}

func (s *DAGServiceWithSpaceID) Add(ctx context.Context, node ipld.Node) error {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.Add(ctx, node)
}

func (s *DAGServiceWithSpaceID) AddMany(ctx context.Context, nodes []ipld.Node) error {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.AddMany(ctx, nodes)
}

func (s *DAGServiceWithSpaceID) Remove(ctx context.Context, cid cid.Cid) error {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.Remove(ctx, cid)
}

func (s *DAGServiceWithSpaceID) RemoveMany(ctx context.Context, cids []cid.Cid) error {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.RemoveMany(ctx, cids)
}
