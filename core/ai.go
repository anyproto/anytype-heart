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

func (mw *Middleware) AIListSummary(ctx context.Context, req *pb.RpcAIListSummaryRequest) *pb.RpcAIListSummaryResponse {
	aiService := mustService[ai.AI](mw)

	objectId, err := aiService.ListSummary(ctx, req)
	code := mapErrorCode(nil,
		errToCode(ai.ErrRateLimitExceeded, pb.RpcAIListSummaryResponseError_RATE_LIMIT_EXCEEDED),
		errToCode(ai.ErrEndpointNotReachable, pb.RpcAIListSummaryResponseError_ENDPOINT_NOT_REACHABLE),
		errToCode(ai.ErrModelNotFound, pb.RpcAIListSummaryResponseError_MODEL_NOT_FOUND),
		errToCode(ai.ErrAuthRequired, pb.RpcAIListSummaryResponseError_AUTH_REQUIRED))

	r := &pb.RpcAIListSummaryResponse{
		Error: &pb.RpcAIListSummaryResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		ObjectId: objectId,
	}
	return r
}

func (mw *Middleware) AIObjectCreateFromUrl(ctx context.Context, req *pb.RpcAIObjectCreateFromUrlRequest) *pb.RpcAIObjectCreateFromUrlResponse {
	aiService := mustService[ai.AI](mw)

	objectId, details, err := aiService.CreateObjectFromUrl(ctx, req.Config, req.Details, req.SpaceId, req.Url)
	code := mapErrorCode(nil,
		errToCode(ai.ErrRateLimitExceeded, pb.RpcAIObjectCreateFromUrlResponseError_RATE_LIMIT_EXCEEDED),
		errToCode(ai.ErrEndpointNotReachable, pb.RpcAIObjectCreateFromUrlResponseError_ENDPOINT_NOT_REACHABLE),
		errToCode(ai.ErrModelNotFound, pb.RpcAIObjectCreateFromUrlResponseError_MODEL_NOT_FOUND),
		errToCode(ai.ErrAuthRequired, pb.RpcAIObjectCreateFromUrlResponseError_AUTH_REQUIRED))

	r := &pb.RpcAIObjectCreateFromUrlResponse{
		Error: &pb.RpcAIObjectCreateFromUrlResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		ObjectId: objectId,
		Details:  details.ToProto(),
	}
	return r
}
