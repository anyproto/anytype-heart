package filestorage

import (
	"context"
	"fmt"
	"github.com/anytypeio/any-sync/commonfile/fileblockstore"
	"github.com/anytypeio/any-sync/commonfile/fileproto"
	"github.com/anytypeio/any-sync/commonfile/fileproto/fileprotoerr"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/badgerfilestore"
	"github.com/anytypeio/go-anytype-middleware/space/storage"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	"go.uber.org/zap"
)

type rpcHandler struct {
	store        badgerfilestore.FileStore
	spaceStorage storage.ClientStorage
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
	c, err := cid.Cast(req.Cid)
	if err != nil {
		return nil, err
	}
	b, err := blocks.NewBlockWithCid(req.Data, c)
	if err != nil {
		return nil, err
	}
	if err = r.store.Add(fileblockstore.CtxWithSpaceId(ctx, req.SpaceId), []blocks.Block{b}); err != nil {
		log.Warn("can't add to store", zap.Error(err))
		return nil, fileprotoerr.ErrUnexpected
	}
	return &fileproto.BlockPushResponse{}, nil
}

func (r *rpcHandler) BlocksDelete(ctx context.Context, req *fileproto.BlocksDeleteRequest) (*fileproto.BlocksDeleteResponse, error) {
	for _, cd := range req.Cids {
		c, err := cid.Cast(cd)
		if err == nil {
			if err = r.store.Delete(fileblockstore.CtxWithSpaceId(ctx, req.SpaceId), c); err != nil {
				log.Warn("can't delete from store", zap.Error(err))
				return nil, err
			}
		}
	}
	return &fileproto.BlocksDeleteResponse{}, nil
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
	// TODO:
	return nil, fmt.Errorf("not implemented")
}

func (r *rpcHandler) Check(ctx context.Context, request *fileproto.CheckRequest) (resp *fileproto.CheckResponse, err error) {
	resp = &fileproto.CheckResponse{
		AllowWrite: true,
	}
	log.Debug("spaceIds requested")
	resp.SpaceIds, err = r.spaceStorage.AllSpaceIds()
	return
}
