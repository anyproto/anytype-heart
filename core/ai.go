package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/ai"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) AIWritingTools(ctx context.Context, req *pb.RpcAIWritingToolsRequest) *pb.RpcAIWritingToolsResponse {
	aiService := mustService[ai.AI](mw)

	result, err := aiService.WritingTools(ctx, req)
	code := mapErrorCode(nil,
		errToCode(ai.ErrRateLimitExceeded, pb.RpcAIWritingToolsResponseError_RATE_LIMIT_EXCEEDED),
		errToCode(ai.ErrEndpointNotReachable, pb.RpcAIWritingToolsResponseError_ENDPOINT_NOT_REACHABLE),
		errToCode(ai.ErrModelNotFound, pb.RpcAIWritingToolsResponseError_MODEL_NOT_FOUND),
		errToCode(ai.ErrAuthRequired, pb.RpcAIWritingToolsResponseError_AUTH_REQUIRED),
		errToCode(ai.ErrUnsupportedLanguage, pb.RpcAIWritingToolsResponseError_LANGUAGE_NOT_SUPPORTED))

	r := &pb.RpcAIWritingToolsResponse{
		Error: &pb.RpcAIWritingToolsResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		Text: result.Answer,
	}

	return r
}

func (mw *Middleware) AIAutofill(ctx context.Context, req *pb.RpcAIAutofillRequest) *pb.RpcAIAutofillResponse {
	aiService := mustService[ai.AI](mw)

	result, err := aiService.Autofill(ctx, req)
	code := mapErrorCode(nil,
		errToCode(ai.ErrRateLimitExceeded, pb.RpcAIAutofillResponseError_RATE_LIMIT_EXCEEDED),
		errToCode(ai.ErrEndpointNotReachable, pb.RpcAIAutofillResponseError_ENDPOINT_NOT_REACHABLE),
		errToCode(ai.ErrModelNotFound, pb.RpcAIAutofillResponseError_MODEL_NOT_FOUND),
		errToCode(ai.ErrAuthRequired, pb.RpcAIAutofillResponseError_AUTH_REQUIRED))

	r := &pb.RpcAIAutofillResponse{
		Error: &pb.RpcAIAutofillResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		// TODO: return slice instead of string
		Text: result.Choices[0],
	}
	return r
}

func (mw *Middleware) AIWebsiteProcess(ctx context.Context, req *pb.RpcAIWebsiteProcessRequest) *pb.RpcAIWebsiteProcessResponse {
	aiService := mustService[ai.AI](mw)

	objectId, err := aiService.WebsiteProcessWithObjectCreate(ctx, req)
	code := mapErrorCode(nil,
		errToCode(ai.ErrRateLimitExceeded, pb.RpcAIWebsiteProcessResponseError_RATE_LIMIT_EXCEEDED),
		errToCode(ai.ErrEndpointNotReachable, pb.RpcAIWebsiteProcessResponseError_ENDPOINT_NOT_REACHABLE),
		errToCode(ai.ErrModelNotFound, pb.RpcAIWebsiteProcessResponseError_MODEL_NOT_FOUND),
		errToCode(ai.ErrAuthRequired, pb.RpcAIWebsiteProcessResponseError_AUTH_REQUIRED))

	r := &pb.RpcAIWebsiteProcessResponse{
		Error: &pb.RpcAIWebsiteProcessResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		ObjectId: objectId,
	}
	return r
}
