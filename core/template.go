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
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	templateId, err := getService[template.Service](mw).TemplateCreateFromObject(ctx, req.ContextId)
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
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	templateId, err := getService[template.Service](mw).TemplateClone(req.SpaceId, req.ContextId)
	return response(templateId, err)
}

func (mw *Middleware) ObjectApplyTemplate(_ context.Context, req *pb.RpcObjectApplyTemplateRequest) *pb.RpcObjectApplyTemplateResponse {
	response := func(err error) *pb.RpcObjectApplyTemplateResponse {
		m := &pb.RpcObjectApplyTemplateResponse{
			Error: &pb.RpcObjectApplyTemplateResponseError{Code: pb.RpcObjectApplyTemplateResponseError_NULL},
		}
		if err != nil {
			m.Error.Code = pb.RpcObjectApplyTemplateResponseError_UNKNOWN_ERROR
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := getService[template.Service](mw).ObjectApplyTemplate(req.ContextId, req.TemplateId)
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
			res.Error.Description = getErrorDescription(err)
			return
		} else {
			res.Path = path
		}
		return res
	}
	path, err := getService[template.Service](mw).TemplateExportAll(ctx, req.Path)
	return response(path, err)
}
