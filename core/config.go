package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

func (mw *Middleware) ConfigGet(*pb.RpcConfigGetRequest) *pb.RpcConfigGetResponse {
	mw.m.RLock()
	defer mw.m.RUnlock()

	if mw.app == nil {
		return &pb.RpcConfigGetResponse{Error: &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NODE_NOT_STARTED, "account not started"}}
	}
	at := mw.app.MustComponent(core.CName).(core.Service)
	return &pb.RpcConfigGetResponse{
		Error:          &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NULL, ""},
		HomeBlockId:    at.PredefinedBlocks().Home,
		ArchiveBlockId: at.PredefinedBlocks().Archive,
		ProfileBlockId: at.PredefinedBlocks().Profile,
		GatewayUrl:     mw.gatewayAddr,
	}
}
