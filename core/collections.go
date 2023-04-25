package core

import (
	"context"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) ObjectCollectionAdd(cctx context.Context, req *pb.RpcObjectCollectionAddRequest) *pb.RpcObjectCollectionAddResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectCollectionAddResponseErrorCode, err error) *pb.RpcObjectCollectionAddResponse {
		m := &pb.RpcObjectCollectionAddResponse{Error: &pb.RpcObjectCollectionAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doCollectionService(func(cs *collection.Service) (err error) {
		return cs.Add(ctx, req)
	})
	if err != nil {
		return response(pb.RpcObjectCollectionAddResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectCollectionAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectCollectionRemove(cctx context.Context, req *pb.RpcObjectCollectionRemoveRequest) *pb.RpcObjectCollectionRemoveResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectCollectionRemoveResponseErrorCode, err error) *pb.RpcObjectCollectionRemoveResponse {
		m := &pb.RpcObjectCollectionRemoveResponse{Error: &pb.RpcObjectCollectionRemoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doCollectionService(func(cs *collection.Service) (err error) {
		return cs.Remove(ctx, req)
	})
	if err != nil {
		return response(pb.RpcObjectCollectionRemoveResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectCollectionRemoveResponseError_NULL, nil)
}

func (mw *Middleware) ObjectCollectionSort(cctx context.Context, req *pb.RpcObjectCollectionSortRequest) *pb.RpcObjectCollectionSortResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectCollectionSortResponseErrorCode, err error) *pb.RpcObjectCollectionSortResponse {
		m := &pb.RpcObjectCollectionSortResponse{Error: &pb.RpcObjectCollectionSortResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doCollectionService(func(cs *collection.Service) (err error) {
		return cs.Sort(ctx, req)
	})
	if err != nil {
		return response(pb.RpcObjectCollectionSortResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectCollectionSortResponseError_NULL, nil)
}

func (mw *Middleware) ObjectToCollection(cctx context.Context, req *pb.RpcObjectToCollectionRequest) *pb.RpcObjectToCollectionResponse {
	response := func(colID string, err error) *pb.RpcObjectToCollectionResponse {
		resp := &pb.RpcObjectToCollectionResponse{
			CollectionId: colID,
			Error: &pb.RpcObjectToCollectionResponseError{
				Code: pb.RpcObjectToCollectionResponseError_NULL,
			},
		}
		if err != nil {
			resp.Error.Code = pb.RpcObjectToCollectionResponseError_UNKNOWN_ERROR
			resp.Error.Description = err.Error()
		}
		return resp
	}
	var (
		setId string
		err   error
	)
	err = mw.doBlockService(func(bs *block.Service) error {
		if setId, err = bs.ObjectToCollection(req.ContextId); err != nil {
			return err
		}
		return nil
	})
	return response(setId, err)
}
