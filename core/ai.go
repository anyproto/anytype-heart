package core

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/ai"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) AIWritingTools(cctx context.Context, req *pb.RpcAIWritingToolsRequest) *pb.RpcAIWritingToolsResponse {
	response := func(resp string, err error) *pb.RpcAIWritingToolsResponse {
		m := &pb.RpcAIWritingToolsResponse{
			Error: &pb.RpcAIWritingToolsResponseError{Code: pb.RpcAIWritingToolsResponseError_NULL},
			Text:  resp,
		}
		if err != nil {
			m.Error.Code = mapErrorCode(err,
				errToCode(ai.ErrRateLimitExceeded, pb.RpcAIWritingToolsResponseError_RATE_LIMIT_EXCEEDED),
				errToCode(ai.ErrEndpointNotReachable, pb.RpcAIWritingToolsResponseError_ENDPOINT_NOT_REACHABLE),
				errToCode(ai.ErrModelNotFound, pb.RpcAIWritingToolsResponseError_MODEL_NOT_FOUND),
				errToCode(ai.ErrAuthRequired, pb.RpcAIWritingToolsResponseError_AUTH_REQUIRED),
				errToCode(ai.ErrUnsupportedLanguage, pb.RpcAIWritingToolsResponseError_LANGUAGE_NOT_SUPPORTED))
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}

	aiService := mw.applicationService.GetApp().Component(ai.CName).(ai.AI)
	if aiService == nil {
		return response("", fmt.Errorf("node not started"))
	}

	result, err := aiService.WritingTools(cctx, req)
	return response(result.Answer, err)
}

func (mw *Middleware) AIAutofill(cctx context.Context, req *pb.RpcAIAutofillRequest) *pb.RpcAIAutofillResponse {
	response := func(resp string, err error) *pb.RpcAIAutofillResponse {
		m := &pb.RpcAIAutofillResponse{
			Error: &pb.RpcAIAutofillResponseError{Code: pb.RpcAIAutofillResponseError_NULL},
			Text:  resp,
		}
		if err != nil {
			m.Error.Code = mapErrorCode(err,
				errToCode(ai.ErrRateLimitExceeded, pb.RpcAIAutofillResponseError_RATE_LIMIT_EXCEEDED),
				errToCode(ai.ErrEndpointNotReachable, pb.RpcAIAutofillResponseError_ENDPOINT_NOT_REACHABLE),
				errToCode(ai.ErrModelNotFound, pb.RpcAIAutofillResponseError_MODEL_NOT_FOUND),
				errToCode(ai.ErrAuthRequired, pb.RpcAIAutofillResponseError_AUTH_REQUIRED))
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}

	aiService := mw.applicationService.GetApp().Component(ai.CName).(ai.AI)
	if aiService == nil {
		return response("", fmt.Errorf("node not started"))
	}

	result, err := aiService.Autofill(cctx, req)
	return response(result.Answer, err)
}
