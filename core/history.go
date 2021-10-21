package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/history"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) HistoryShow(req *pb.RpcHistoryShowRequest) *pb.RpcHistoryShowResponse {
	response := func(show *pb.EventObjectShow, ver *pb.RpcHistoryVersionsVersion, err error) (res *pb.RpcHistoryShowResponse) {
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
			res.ObjectShow = show
			res.Version = ver
			res.TraceId = req.TraceId
		}
		return res
	}
	var (
		show *pb.EventObjectShow
		ver  *pb.RpcHistoryVersionsVersion
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
