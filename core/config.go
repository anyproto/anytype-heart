package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) ConfigGet(*pb.RpcConfigGetRequest) *pb.RpcConfigGetResponse {
	at := mw.GetAnytype()
	if at == nil {
		return &pb.RpcConfigGetResponse{Error: &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NODE_NOT_STARTED, "account not started"}}
	}
	pBlocks := at.PredefinedBlocks()
	return &pb.RpcConfigGetResponse{
		Error:                 &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NULL, ""},
		HomeBlockId:           pBlocks.Home,
		ArchiveBlockId:        pBlocks.Archive,
		ProfileBlockId:        pBlocks.Profile,
		MarketplaceTypeId:     pBlocks.MarketplaceType,
		MarketplaceRelationId: pBlocks.MarketplaceRelation,
		GatewayUrl:            mw.gatewayAddr,
	}
}
