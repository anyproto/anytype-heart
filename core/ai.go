package core

import (
	"context"

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
