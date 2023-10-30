package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) TemplateCreateFromObject(ctx context.Context, req *pb.RpcTemplateCreateFromObjectRequest) *pb.RpcTemplateCreateFromObjectResponse {
	response := func(templateId string, err error) *pb.RpcTemplateCreateFromObjectResponse {
		m := &pb.RpcTemplateCreateFromObjectResponse{
			Error: &pb.RpcTemplateCreateFromObjectResponseError{Code: pb.RpcTemplateCreateFromObjectResponseError_NULL},
			Id:    templateId,
		}
		if err != nil {
			m.Error.Code = pb.RpcTemplateCreateFromObjectResponseError_UNKNOWN_ERROR
			m.Error.Description = err.Error()
		}
		return m
	}
	var templateId string
	err := mw.doTemplateService(func(ts template.Service) (err error) {
		templateId, err = ts.TemplateCreateFromObject(ctx, req.ContextId)
		return
	})
	return response(templateId, err)
}

func (mw *Middleware) TemplateClone(_ context.Context, req *pb.RpcTemplateCloneRequest) *pb.RpcTemplateCloneResponse {
	response := func(templateId string, err error) *pb.RpcTemplateCloneResponse {
		m := &pb.RpcTemplateCloneResponse{
			Error: &pb.RpcTemplateCloneResponseError{Code: pb.RpcTemplateCloneResponseError_NULL},
			Id:    templateId,
		}
		if err != nil {
			m.Error.Code = pb.RpcTemplateCloneResponseError_UNKNOWN_ERROR
			m.Error.Description = err.Error()
		}
		return m
	}
	var templateId string
	err := mw.doTemplateService(func(ts template.Service) (err error) {
		templateId, err = ts.TemplateClone(req.SpaceId, req.ContextId)
		return
	})
	return response(templateId, err)
}

func (mw *Middleware) ObjectApplyTemplate(cctx context.Context, req *pb.RpcObjectApplyTemplateRequest) *pb.RpcObjectApplyTemplateResponse {
	response := func(err error) *pb.RpcObjectApplyTemplateResponse {
		m := &pb.RpcObjectApplyTemplateResponse{
			Error: &pb.RpcObjectApplyTemplateResponseError{Code: pb.RpcObjectApplyTemplateResponseError_NULL},
		}
		if err != nil {
			m.Error.Code = pb.RpcObjectApplyTemplateResponseError_UNKNOWN_ERROR
			m.Error.Description = err.Error()
		}
		return m
	}
	err := mw.doTemplateService(func(ts template.Service) (err error) {
		return ts.ObjectApplyTemplate(req.ContextId, req.TemplateId)
	})
	return response(err)
}

func (mw *Middleware) TemplateExportAll(ctx context.Context, req *pb.RpcTemplateExportAllRequest) *pb.RpcTemplateExportAllResponse {
	response := func(path string, err error) (res *pb.RpcTemplateExportAllResponse) {
		res = &pb.RpcTemplateExportAllResponse{
			Error: &pb.RpcTemplateExportAllResponseError{
				Code: pb.RpcTemplateExportAllResponseError_NULL,
			},
		}
		if err != nil {
			res.Error.Code = pb.RpcTemplateExportAllResponseError_UNKNOWN_ERROR
			res.Error.Description = err.Error()
			return
		} else {
			res.Path = path
		}
		return res
	}
	var (
		path string
	)
	err := mw.doTemplateService(func(ts template.Service) error {
		return ts.TemplateExportAll(ctx, req.Path)
	})
	return response(path, err)
}

// WorkspaceExport is unused now, it must be fixed if someone wants to use it
func (mw *Middleware) WorkspaceExport(cctx context.Context, req *pb.RpcWorkspaceExportRequest) *pb.RpcWorkspaceExportResponse {
	return &pb.RpcWorkspaceExportResponse{
		Error: &pb.RpcWorkspaceExportResponseError{
			Code:        pb.RpcWorkspaceExportResponseError_NULL,
			Description: "Not implemented",
		},
	}
}
