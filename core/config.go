package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) ConfigGet(*pb.RpcConfigGetRequest) *pb.RpcConfigGetResponse {
	mw.m.RLock()
	defer mw.m.RUnlock()

	if mw.Anytype == nil {
		return &pb.RpcConfigGetResponse{Error: &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NODE_NOT_STARTED, "account not started"}}
	}

	return &pb.RpcConfigGetResponse{
		Error:          &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NULL, ""},
		HomeBlockId:    mw.Anytype.PredefinedBlocks().Home,
		ArchiveBlockId: mw.Anytype.PredefinedBlocks().Archive,
		ProfileBlockId: mw.Anytype.PredefinedBlocks().Profile,
		GatewayUrl:     mw.gatewayAddr,
	}
}
