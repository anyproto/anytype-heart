package core

import (
	"context"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/history"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func (mw *Middleware) HistoryShowVersion(cctx context.Context, req *pb.RpcHistoryShowVersionRequest) *pb.RpcHistoryShowVersionResponse {
	response := func(obj *model.ObjectView, ver *pb.RpcHistoryVersion, err error) (res *pb.RpcHistoryShowVersionResponse) {
		res = &pb.RpcHistoryShowVersionResponse{
			Error: &pb.RpcHistoryShowVersionResponseError{
				Code: pb.RpcHistoryShowVersionResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcHistoryShowVersionResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.ObjectView = obj
			res.Version = ver
			res.TraceId = req.TraceId
		}
		return res
	}
	var (
		obj *model.ObjectView
		ver *pb.RpcHistoryVersion
		err error
	)
	if err = mw.doBlockService(func(bs block.Service) (err error) {
		hs := mw.app.MustComponent(history.CName).(history.History)
		obj, ver, err = hs.Show(req.ObjectId, req.VersionId)
		return
	}); err != nil {
		return response(nil, nil, err)
	}

	return response(obj, ver, nil)
}

func (mw *Middleware) HistoryGetVersions(cctx context.Context, req *pb.RpcHistoryGetVersionsRequest) *pb.RpcHistoryGetVersionsResponse {
	response := func(vers []*pb.RpcHistoryVersion, err error) (res *pb.RpcHistoryGetVersionsResponse) {
		res = &pb.RpcHistoryGetVersionsResponse{
			Error: &pb.RpcHistoryGetVersionsResponseError{
				Code: pb.RpcHistoryGetVersionsResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcHistoryGetVersionsResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.Versions = vers
		}
		return res
	}
	var (
		vers []*pb.RpcHistoryVersion
		err  error
	)
	if err = mw.doBlockService(func(bs block.Service) (err error) {
		hs := mw.app.MustComponent(history.CName).(history.History)
		vers, err = hs.Versions(req.ObjectId, req.LastVersionId, int(req.Limit))
		return
	}); err != nil {
		return response(nil, err)
	}
	return response(vers, nil)
}

func (mw *Middleware) HistorySetVersion(cctx context.Context, req *pb.RpcHistorySetVersionRequest) *pb.RpcHistorySetVersionResponse {
	response := func(err error) (res *pb.RpcHistorySetVersionResponse) {
		res = &pb.RpcHistorySetVersionResponse{
			Error: &pb.RpcHistorySetVersionResponseError{
				Code: pb.RpcHistorySetVersionResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcHistorySetVersionResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		}
		return
	}
	return response(mw.doBlockService(func(bs block.Service) (err error) {
		hs := mw.app.MustComponent(history.CName).(history.History)
		return hs.SetVersion(req.ObjectId, req.VersionId)
	}))
}
