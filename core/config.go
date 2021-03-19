package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/gateway"
)

func (mw *Middleware) ConfigGet(*pb.RpcConfigGetRequest) *pb.RpcConfigGetResponse {
	mw.m.RLock()
	defer mw.m.RUnlock()
	if mw.app == nil {
		return &pb.RpcConfigGetResponse{Error: &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NODE_NOT_STARTED, "account not started"}}
	}
	at := mw.app.MustComponent(core.CName).(core.Service)
	gwAddr := mw.app.MustComponent(gateway.CName).(gateway.Gateway).Addr()

	if gwAddr != "" {
		gwAddr = "http://" + gwAddr
	}

	pBlocks := at.PredefinedBlocks()
	return &pb.RpcConfigGetResponse{
		Error:                 &pb.RpcConfigGetResponseError{pb.RpcConfigGetResponseError_NULL, ""},
		HomeBlockId:           pBlocks.Home,
		ArchiveBlockId:        pBlocks.Archive,
		ProfileBlockId:        pBlocks.Profile,
		MarketplaceTypeId:     pBlocks.MarketplaceType,
		MarketplaceRelationId: pBlocks.MarketplaceRelation,
		GatewayUrl:            gwAddr,
	}
}
