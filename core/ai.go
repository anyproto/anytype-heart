package core

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/ai/llmclient"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) AIWritingTools(ctx context.Context, req *pb.RpcAIWritingToolsRequest) *pb.RpcAIWritingToolsResponse {

	r := &pb.RpcAIWritingToolsResponse{
		Error: &pb.RpcAIWritingToolsResponseError{
			Code:        pb.RpcAIWritingToolsResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}

	return r
}

func (mw *Middleware) AIAutofill(ctx context.Context, req *pb.RpcAIAutofillRequest) *pb.RpcAIAutofillResponse {
	r := &pb.RpcAIAutofillResponse{
		Error: &pb.RpcAIAutofillResponseError{
			Code:        pb.RpcAIAutofillResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
	return r
}

func (mw *Middleware) AIListSummary(ctx context.Context, req *pb.RpcAIListSummaryRequest) *pb.RpcAIListSummaryResponse {
	r := &pb.RpcAIListSummaryResponse{
		Error: &pb.RpcAIListSummaryResponseError{
			Code:        pb.RpcAIListSummaryResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
	return r
}

func (mw *Middleware) AIObjectCreateFromUrl(ctx context.Context, req *pb.RpcAIObjectCreateFromUrlRequest) *pb.RpcAIObjectCreateFromUrlResponse {
	r := &pb.RpcAIObjectCreateFromUrlResponse{
		Error: &pb.RpcAIObjectCreateFromUrlResponseError{
			Code:        pb.RpcAIObjectCreateFromUrlResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
	return r
}

// AIListModels validates a provider config (base URL + token) and returns
// the models it offers. There is deliberately no separate "validate config"
// RPC: a successful model list already proves the endpoint is reachable and
// the token works, and the error codes below say what went wrong when it
// isn't. OpenAI's catalog is narrowed to chat-completion models before it
// goes back — see llmclient.FilterChatModels for why that needs a heuristic
// and why every other provider is returned unfiltered.
func (mw *Middleware) AIListModels(ctx context.Context, req *pb.RpcAIListModelsRequest) *pb.RpcAIListModelsResponse {
	cfg, err := llmclient.FromProtoForListing(req.GetConfig())
	if err != nil {
		return &pb.RpcAIListModelsResponse{
			Error: &pb.RpcAIListModelsResponseError{
				Code:        pb.RpcAIListModelsResponseError_BAD_INPUT,
				Description: err.Error(),
			},
		}
	}

	models, err := llmclient.ListModels(ctx, cfg)
	if err != nil {
		return &pb.RpcAIListModelsResponse{Error: aiListModelsErrorFromErr(err)}
	}

	if req.GetConfig().GetProvider() == pb.RpcAI_OPENAI {
		models = llmclient.FilterChatModels(models)
	}

	resp := &pb.RpcAIListModelsResponse{
		Error:  &pb.RpcAIListModelsResponseError{Code: pb.RpcAIListModelsResponseError_NULL},
		Models: make([]*pb.RpcAIListModelsModel, 0, len(models)),
	}
	for _, m := range models {
		resp.Models = append(resp.Models, &pb.RpcAIListModelsModel{Id: m.ID, OwnedBy: m.OwnedBy})
	}
	return resp
}

// aiListModelsErrorFromErr maps an llmclient sentinel onto the AI.ListModels
// wire error code. Falls back to UNKNOWN_ERROR for anything unrecognized —
// the description still carries the underlying message.
func aiListModelsErrorFromErr(err error) *pb.RpcAIListModelsResponseError {
	code := pb.RpcAIListModelsResponseError_UNKNOWN_ERROR
	switch {
	case errors.Is(err, llmclient.ErrAuthRequired):
		code = pb.RpcAIListModelsResponseError_AUTH_REQUIRED
	case errors.Is(err, llmclient.ErrRateLimited):
		code = pb.RpcAIListModelsResponseError_RATE_LIMIT_EXCEEDED
	case errors.Is(err, llmclient.ErrModelNotFound):
		code = pb.RpcAIListModelsResponseError_MODEL_NOT_FOUND
	case errors.Is(err, llmclient.ErrEndpointUnreachable):
		code = pb.RpcAIListModelsResponseError_ENDPOINT_NOT_REACHABLE
	}
	return &pb.RpcAIListModelsResponseError{Code: code, Description: err.Error()}
}
