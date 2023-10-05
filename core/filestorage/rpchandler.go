package filestorage

import (
	"context"

	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/space/spacecore/storage"
)

type rpcHandler struct {
	store        *flatStore
	spaceStorage storage.ClientStorage
}

func (r *rpcHandler) FilesDelete(ctx context.Context, request *fileproto.FilesDeleteRequest) (*fileproto.FilesDeleteResponse, error) {
	return nil, fileprotoerr.ErrForbidden
}

func (r *rpcHandler) FilesInfo(ctx context.Context, request *fileproto.FilesInfoRequest) (*fileproto.FilesInfoResponse, error) {
	return nil, fileprotoerr.ErrForbidden

}

func (r *rpcHandler) SpaceInfo(ctx context.Context, request *fileproto.SpaceInfoRequest) (*fileproto.SpaceInfoResponse, error) {
	return nil, fileprotoerr.ErrForbidden

}

func (r *rpcHandler) BlockGet(ctx context.Context, req *fileproto.BlockGetRequest) (resp *fileproto.BlockGetResponse, err error) {
	resp = &fileproto.BlockGetResponse{
		Cid: req.Cid,
	}
	c, err := cid.Cast(req.Cid)
	if err != nil {
		return nil, err
	}
	b, err := r.store.Get(fileblockstore.CtxWithSpaceId(ctx, req.SpaceId), c)
	if err != nil {
		return nil, err
	} else {
		resp.Data = b.RawData()
	}
	return
}

func (r *rpcHandler) BlockPush(ctx context.Context, req *fileproto.BlockPushRequest) (*fileproto.BlockPushResponse, error) {
	return nil, fileprotoerr.ErrForbidden

}

func (r *rpcHandler) BlocksCheck(ctx context.Context, req *fileproto.BlocksCheckRequest) (*fileproto.BlocksCheckResponse, error) {
	cids := make([]cid.Cid, 0, len(req.Cids))
	for _, cd := range req.Cids {
		c, err := cid.Cast(cd)
		if err == nil {
			cids = append(cids, c)
		}
	}
	availability, err := r.store.BlockAvailability(ctx, cids)
	if err != nil {
		return nil, err
	}
	return &fileproto.BlocksCheckResponse{
		BlocksAvailability: availability,
	}, nil
}

func (r *rpcHandler) BlocksBind(ctx context.Context, req *fileproto.BlocksBindRequest) (*fileproto.BlocksBindResponse, error) {
	return nil, fileprotoerr.ErrForbidden
}

func (r *rpcHandler) Check(ctx context.Context, request *fileproto.CheckRequest) (resp *fileproto.CheckResponse, err error) {
	resp = &fileproto.CheckResponse{
		AllowWrite: false,
	}
	resp.SpaceIds, err = r.spaceStorage.AllSpaceIds()
	return
}
