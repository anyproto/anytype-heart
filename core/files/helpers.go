package files

import (
	"context"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
)

type dagServiceWithSpaceID struct {
	spaceID    string
	dagService ipld.DAGService
}

func (s *dagServiceWithSpaceID) Get(ctx context.Context, cid cid.Cid) (ipld.Node, error) {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.Get(ctx, cid)
}

func (s *dagServiceWithSpaceID) GetMany(ctx context.Context, cids []cid.Cid) <-chan *ipld.NodeOption {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.GetMany(ctx, cids)
}

func (s *dagServiceWithSpaceID) Add(ctx context.Context, node ipld.Node) error {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.Add(ctx, node)
}

func (s *dagServiceWithSpaceID) AddMany(ctx context.Context, nodes []ipld.Node) error {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.AddMany(ctx, nodes)
}

func (s *dagServiceWithSpaceID) Remove(ctx context.Context, cid cid.Cid) error {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.Remove(ctx, cid)
}

func (s *dagServiceWithSpaceID) RemoveMany(ctx context.Context, cids []cid.Cid) error {
	ctx = fileblockstore.CtxWithSpaceId(ctx, s.spaceID)
	return s.dagService.RemoveMany(ctx, cids)
}

func (s *service) dagServiceForSpace(spaceID string) ipld.DAGService {
	return &dagServiceWithSpaceID{
		spaceID:    spaceID,
		dagService: s.dagService,
	}
}
