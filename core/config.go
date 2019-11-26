package core

import "github.com/anytypeio/go-anytype-middleware/pb"

func (mw *Middleware) ConfigGet(*pb.RpcConfigGetRequest) *pb.RpcConfigGetResponse {
	var homeBlockId string
	if mw.Anytype != nil {
		homeBlockId = mw.Anytype.PredefinedBlockIds().Home
	}
	return &pb.RpcConfigGetResponse{HomeBlockId: homeBlockId}
}
