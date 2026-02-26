package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/template"
	"github.com/anyproto/anytype-heart/core/domain"
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
	templateId, err := mustService[template.Service](mw).TemplateCreateFromObject(ctx, req.ContextId)
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
	templateId, err := mustService[template.Service](mw).TemplateClone(req.SpaceId, req.ContextId)
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
	err := mustService[template.Service](mw).ObjectApplyTemplate(req.ContextId, req.TemplateId)
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
	path, err := mustService[template.Service](mw).TemplateExportAll(ctx, req.Path)
	return response(path, err)
}

func (mw *Middleware) TemplateSetPlaceholders(cctx context.Context, req *pb.RpcTemplateSetPlaceholdersRequest) *pb.RpcTemplateSetPlaceholdersResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcTemplateSetPlaceholdersResponseErrorCode, err error) *pb.RpcTemplateSetPlaceholdersResponse {
		m := &pb.RpcTemplateSetPlaceholdersResponse{Error: &pb.RpcTemplateSetPlaceholdersResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}

	placeholders := make([]domain.TemplatePlaceholder, 0, len(req.Placeholders))
	for _, p := range req.Placeholders {
		placeholders = append(placeholders, domain.TemplatePlaceholder{
			RelationKey: domain.RelationKey(p.RelationKey),
			Type:        p.Type,
		})
	}

	err := mustService[detailservice.Service](mw).SetTemplatePlaceholders(ctx, req.TemplateId, placeholders)
	if err != nil {
		return response(pb.RpcTemplateSetPlaceholdersResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcTemplateSetPlaceholdersResponseError_NULL, nil)
}
