package core

import (
	"github.com/anytypeio/go-anytype-library/gateway"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) ConfigGet(*pb.RpcConfigGetRequest) *pb.RpcConfigGetResponse {
	var homeBlockId, gatewayUrl string
	if mw.Anytype != nil {
		homeBlockId = mw.Anytype.PredefinedBlockIds().Home
		gatewayUrl = gateway.GatewayAddr()
	}
	return &pb.RpcConfigGetResponse{HomeBlockId: homeBlockId, GatewayUrl: gatewayUrl}
}
