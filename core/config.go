package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/wallet"
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
	wallet := mw.app.MustComponent(wallet.CName).(wallet.Wallet)
	var deviceId string
	deviceKey, err := wallet.GetDevicePrivkey()
	if err == nil {
		deviceId = deviceKey.Address()
	}

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
		MarketplaceTemplateId: pBlocks.MarketplaceTemplate,
		GatewayUrl:            gwAddr,
		DeviceId:              deviceId,
	}
}
