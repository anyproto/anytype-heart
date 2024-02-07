package files

import (
	"context"
	"fmt"
	"io"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	ufsio "github.com/ipfs/boxo/ipld/unixfs/io"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"github.com/ipfs/interface-go-ipfs-core/path"

	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
)

func (s *service) dagServiceForSpace(spaceID string) ipld.DAGService {
	return filehelper.NewDAGServiceWithSpaceID(spaceID, s.dagService)
}

func (s *service) addFileData(ctx context.Context, spaceID string, r io.Reader) (ipld.Node, error) {
	cctx := fileblockstore.CtxWithSpaceId(ctx, spaceID)
	return s.commonFile.AddFile(cctx, r)
}

func (s *service) getFile(ctx context.Context, spaceID string, c cid.Cid) (ufsio.ReadSeekCloser, error) {
	cctx := fileblockstore.CtxWithSpaceId(ctx, spaceID)
	return s.commonFile.GetFile(cctx, c)
}

func (s *service) hasCid(ctx context.Context, spaceID string, c cid.Cid) (bool, error) {
	cctx := fileblockstore.CtxWithSpaceId(ctx, spaceID)
	return s.commonFile.HasCid(cctx, c)
}

func (s *service) dataAtPath(ctx context.Context, spaceID string, pth string) (cid.Cid, symmetric.ReadSeekCloser, error) {
	dagService := s.dagServiceForSpace(spaceID)
	resolvedPath, err := helpers.ResolvePath(ctx, dagService, path.New(pth))
	if err != nil {
		return cid.Undef, nil, fmt.Errorf("failed to resolve path %s: %w", pth, err)
	}

	r, err := s.getFile(ctx, spaceID, resolvedPath.Cid())
	if err != nil {
		return cid.Undef, nil, fmt.Errorf("failed to resolve path %s: %w", pth, err)
	}

	return resolvedPath.Cid(), r, nil
}
