package files

import (
	"context"
	"fmt"
	"io"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	ufsio "github.com/ipfs/boxo/ipld/unixfs/io"
	"github.com/ipfs/boxo/path"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"

	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/pkg/lib/crypto/symmetric"
	"github.com/anyproto/anytype-heart/pkg/lib/ipfs/helpers"
)

func (s *service) dagServiceForSpace(ctx context.Context, spaceID string) ipld.DAGService {
	// Return the default DAG service wrapped with space ID
	// Custom DAG services (like batched) are now passed explicitly via AddOptions
	return filehelper.NewDAGServiceWithSpaceID(spaceID, s.dagService)
}


func (s *service) addFileData(ctx context.Context, spaceID string, r io.Reader, fileHandler *fileservice.FileHandler) (ipld.Node, error) {
	// Use custom FileHandler if provided, otherwise use default
	if fileHandler != nil {
		cctx := fileblockstore.CtxWithSpaceId(ctx, spaceID)
		return fileHandler.AddFile(cctx, r)
	}
	cctx := fileblockstore.CtxWithSpaceId(ctx, spaceID)
	return s.commonFile.AddFile(cctx, r)
}


func (s *service) getFile(ctx context.Context, spaceID string, c cid.Cid) (ufsio.ReadSeekCloser, error) {
	cctx := fileblockstore.CtxWithSpaceId(ctx, spaceID)
	return s.commonFile.GetFile(cctx, c)
}

func (s *service) dataAtPath(ctx context.Context, spaceID string, pth string) (cid.Cid, symmetric.ReadSeekCloser, error) {
	dagService := s.dagServiceForSpace(ctx, spaceID)
	newPath, err := path.NewPath("/ipfs/" + pth)
	if err != nil {
		return cid.Undef, nil, fmt.Errorf("failed to resolve path %s: %w", pth, err)
	}
	rCid, err := helpers.ResolveCid(ctx, dagService, newPath)
	if err != nil {
		return cid.Undef, nil, fmt.Errorf("failed to resolve path %s: %w", pth, err)
	}

	r, err := s.getFile(ctx, spaceID, rCid)
	if err != nil {
		return cid.Undef, nil, fmt.Errorf("failed to resolve path %s: %w", pth, err)
	}

	return rCid, r, nil
}
