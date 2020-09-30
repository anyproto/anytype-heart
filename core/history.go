package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) HistoryShow(req *pb.RpcHistoryShowRequest) *pb.RpcHistoryShowResponse {
	response := func(show *pb.EventBlockShow, ver *pb.RpcHistoryVersionsVersion, err error) (res *pb.RpcHistoryShowResponse) {
		res = &pb.RpcHistoryShowResponse{
			Error: &pb.RpcHistoryShowResponseError{
				Code: pb.RpcHistoryShowResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcHistoryShowResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.BlockShow = show
			res.Version = ver
		}
		return res
	}
	var (
		show *pb.EventBlockShow
		ver  *pb.RpcHistoryVersionsVersion
		err  error
	)
	if err = mw.doBlockService(func(bs block.Service) (err error) {
		show, ver, err = bs.History().Show(req.PageId, req.VersionId)
		return
	}); err != nil {
		return response(nil, nil, err)
	}
	return response(show, ver, nil)
}

func (mw *Middleware) HistoryVersions(req *pb.RpcHistoryVersionsRequest) *pb.RpcHistoryVersionsResponse {
	response := func(vers []*pb.RpcHistoryVersionsVersion, err error) (res *pb.RpcHistoryVersionsResponse) {
		res = &pb.RpcHistoryVersionsResponse{
			Error: &pb.RpcHistoryVersionsResponseError{
				Code: pb.RpcHistoryVersionsResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcHistoryVersionsResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.Versions = vers
		}
		return res
	}
	var (
		vers []*pb.RpcHistoryVersionsVersion
		err  error
	)
	if err = mw.doBlockService(func(bs block.Service) (err error) {
		vers, err = bs.History().Versions(req.PageId, req.LastVersionId, int(req.Limit))
		return
	}); err != nil {
		return response(nil, err)
	}
	return response(vers, nil)
}

func (mw *Middleware) HistorySetVersion(req *pb.RpcHistorySetVersionRequest) *pb.RpcHistorySetVersionResponse {
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
		return bs.History().SetVersion(req.PageId, req.VersionId)
	}))
}
