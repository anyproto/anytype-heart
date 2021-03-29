package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) MakeTemplate(req *pb.RpcMakeTemplateRequest) *pb.RpcMakeTemplateResponse {
	response := func(templateId string, err error) *pb.RpcMakeTemplateResponse {
		m := &pb.RpcMakeTemplateResponse{
			Error: &pb.RpcMakeTemplateResponseError{Code: pb.RpcMakeTemplateResponseError_NULL},
			Id:    templateId,
		}
		if err != nil {
			m.Error.Code = pb.RpcMakeTemplateResponseError_UNKNOWN_ERROR
			m.Error.Description = err.Error()
		}
		return m
	}
	var templateId string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		templateId, err = bs.MakeTemplate(req.ContextId)
		return
	})
	return response(templateId, err)
}
