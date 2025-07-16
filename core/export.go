package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ObjectListExport(cctx context.Context, req *pb.RpcObjectListExportRequest) *pb.RpcObjectListExportResponse {
	response := func(path string, succeed int, err error) (res *pb.RpcObjectListExportResponse) {
		res = &pb.RpcObjectListExportResponse{
			Error: &pb.RpcObjectListExportResponseError{
				Code: pb.RpcObjectListExportResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcObjectListExportResponseError_UNKNOWN_ERROR
			res.Error.Description = getErrorDescription(err)
			return
		} else {
			res.Path = path
			res.Succeed = int32(succeed)
		}
		return res
	}
	var (
		path    string
		succeed int
		err     error
	)
	err = mw.doBlockService(func(_ *block.Service) error {
		es := mw.applicationService.GetApp().MustComponent(export.CName).(export.Export)
		path, succeed, err = es.Export(cctx, *req)
		return err
	})
	return response(path, succeed, err)
}

func (mw *Middleware) ObjectExport(cctx context.Context, req *pb.RpcObjectExportRequest) *pb.RpcObjectExportResponse {
	response := func(result string, err error) (res *pb.RpcObjectExportResponse) {
		res = &pb.RpcObjectExportResponse{
			Error: &pb.RpcObjectExportResponseError{
				Code: pb.RpcObjectExportResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcObjectExportResponseError_UNKNOWN_ERROR
			res.Error.Description = getErrorDescription(err)
			return
		} else {
			res.Result = result
		}
		return res
	}
	var (
		result string
		err    error
	)
	err = mw.doBlockService(func(_ *block.Service) error {
		es := mw.applicationService.GetApp().MustComponent(export.CName).(export.Export)
		result, err = es.ExportSingleInMemory(cctx, req.SpaceId, req.ObjectId, req.Format)
		return err
	})
	return response(result, err)
}
