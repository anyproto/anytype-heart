package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) ConfigGet(*pb.RpcConfigGetRequest) *pb.RpcConfigGetResponse {
	if mw.Anytype == nil {
		return &pb.RpcConfigGetResponse{Error: &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NODE_NOT_STARTED, "account not started"}}
	}

	return &pb.RpcConfigGetResponse{
		Error:          &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NULL, ""},
		HomeBlockId:    mw.Anytype.PredefinedBlockIds().Home,
		ArchiveBlockId: mw.Anytype.PredefinedBlockIds().Archive,
		GatewayUrl:     mw.gatewayAddr,
	}
}
