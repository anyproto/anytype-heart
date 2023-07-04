package files

import (
	"context"
	"fmt"
	"io"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	ufsio "github.com/ipfs/go-unixfs/io"
	"github.com/ipfs/interface-go-ipfs-core/path"

	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
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

func (s *service) addFile(ctx session.Context, r io.Reader) (ipld.Node, error) {
	cctx := fileblockstore.CtxWithSpaceId(ctx.Context(), ctx.SpaceID())
	return s.commonFile.AddFile(cctx, r)
}

func (s *service) getFile(ctx session.Context, c cid.Cid) (ufsio.ReadSeekCloser, error) {
	cctx := fileblockstore.CtxWithSpaceId(ctx.Context(), ctx.SpaceID())
	return s.commonFile.GetFile(cctx, c)
}

func (s *service) hasCid(ctx session.Context, c cid.Cid) (bool, error) {
	cctx := fileblockstore.CtxWithSpaceId(ctx.Context(), ctx.SpaceID())
	return s.commonFile.HasCid(cctx, c)
}

func (s *service) dataAtPath(ctx session.Context, pth string) (cid.Cid, symmetric.ReadSeekCloser, error) {
	dagService := s.dagServiceForSpace(ctx.SpaceID())
	resolvedPath, err := helpers.ResolvePath(ctx.Context(), dagService, path.New(pth))
	if err != nil {
		return cid.Undef, nil, fmt.Errorf("failed to resolve path %s: %w", pth, err)
	}

	r, err := s.getFile(ctx, resolvedPath.Cid())
	if err != nil {
		return cid.Undef, nil, fmt.Errorf("failed to resolve path %s: %w", pth, err)
	}

	return resolvedPath.Cid(), r, nil
}
