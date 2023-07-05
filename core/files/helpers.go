package files

import (
	"fmt"
	"io"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	ufsio "github.com/ipfs/go-unixfs/io"
	"github.com/ipfs/interface-go-ipfs-core/path"

	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
)

func (s *service) dagServiceForSpace(spaceID string) ipld.DAGService {
	return filehelper.NewDAGServiceWithSpaceID(spaceID, s.dagService)
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
