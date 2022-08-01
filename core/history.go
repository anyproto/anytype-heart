package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/history"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) HistoryShowVersion(req *pb.RpcHistoryShowVersionRequest) *pb.RpcHistoryShowVersionResponse {
	response := func(show *pb.EventObjectShow, ver *pb.RpcHistoryVersion, err error) (res *pb.RpcHistoryShowVersionResponse) {
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
			res.ObjectShow = show
			res.Version = ver
			res.TraceId = req.TraceId
		}
		return res
	}
	var (
		show *pb.EventObjectShow
		ver  *pb.RpcHistoryVersion
		err  error
	)
	if err = mw.doBlockService(func(bs block.Service) (err error) {
		hs := mw.app.MustComponent(history.CName).(history.History)
		show, ver, err = hs.Show(req.PageId, req.VersionId)
		return
	}); err != nil {
		return response(nil, nil, err)
	}

	return response(show, ver, nil)
}

func (mw *Middleware) HistoryGetVersions(req *pb.RpcHistoryGetVersionsRequest) *pb.RpcHistoryGetVersionsResponse {
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
		vers, err = hs.Versions(req.PageId, req.LastVersionId, int(req.Limit))
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
		hs := mw.app.MustComponent(history.CName).(history.History)
		return hs.SetVersion(req.PageId, req.VersionId)
	}))
}
