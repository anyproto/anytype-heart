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
	pBlocks := mw.Anytype.PredefinedBlocks()
	return &pb.RpcConfigGetResponse{
		Error:                &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NULL, ""},
		HomeBlockId:          pBlocks.Home,
		ArchiveBlockId:       pBlocks.Archive,
		ProfileBlockId:       pBlocks.Profile,
		MarketplaceId:        pBlocks.Marketplace,
		MarketplaceLibraryId: pBlocks.MarketplaceLibrary,
		GatewayUrl:           mw.gatewayAddr,
	}
}
