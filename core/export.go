package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/export"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) Export(req *pb.RpcExportRequest) *pb.RpcExportResponse {
	response := func(path string, err error) (res *pb.RpcExportResponse) {
		res = &pb.RpcExportResponse{
			Error: &pb.RpcExportResponseError{
				Code: pb.RpcExportResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcExportResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.Path = path
		}
		return res
	}
	var (
		path string
		err  error
	)
	err = mw.doBlockService(func(_ block.Service) error {
		es := mw.app.MustComponent(export.CName).(export.Export)
		path, err = es.Export(*req)
		return err
	})
	return response(path, err)
}
