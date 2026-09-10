package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/export"
	"github.com/anyproto/anytype-heart/core/block/export/report"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (mw *Middleware) ObjectListExport(cctx context.Context, req *pb.RpcObjectListExportRequest) *pb.RpcObjectListExportResponse {
	response := func(path string, diagnostics *model.ExportReport, err error) (res *pb.RpcObjectListExportResponse) {
		if diagnostics == nil {
			diagnostics = new(report.Collector).Snapshot(err)
		}
		res = &pb.RpcObjectListExportResponse{
			Report:  diagnostics,
			Succeed: diagnostics.GetSucceed(),
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
		}
		return res
	}
	var (
		path        string
		diagnostics *model.ExportReport
		err         error
	)
	err = mw.doBlockService(func(_ *block.Service) error {
		es := mw.applicationService.GetApp().MustComponent(export.CName).(export.Export)
		path, diagnostics, err = es.Export(cctx, *req)
		return err
	})
	return response(path, diagnostics, err)
}

func (mw *Middleware) ObjectExport(cctx context.Context, req *pb.RpcObjectExportRequest) *pb.RpcObjectExportResponse {
	response := func(result string, diagnostics *model.ExportReport, err error) (res *pb.RpcObjectExportResponse) {
		if diagnostics == nil {
			diagnostics = new(report.Collector).Snapshot(err)
		}
		res = &pb.RpcObjectExportResponse{
			Report: diagnostics,
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
		result      string
		diagnostics *model.ExportReport
		err         error
	)
	err = mw.doBlockService(func(_ *block.Service) error {
		es := mw.applicationService.GetApp().MustComponent(export.CName).(export.Export)
		result, diagnostics, err = es.ExportSingleInMemory(cctx, req.SpaceId, req.ObjectId, req.Format)
		return err
	})
	return response(result, diagnostics, err)
}
