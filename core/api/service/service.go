package service

import (
	"context"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("api-internal-service")

type Service struct {
	mw          apicore.ClientCommands
	gatewayUrl  string
	techSpaceId string

	subscriptionService  subscription.Service
	crossSpaceSubService crossspacesub.Service
	componentCtx         context.Context
	componentCtxCancel   context.CancelFunc

	cache         *cacheManager
	subscriptions *subscriptions
}

func NewService(mw apicore.ClientCommands, gatewayUrl string, techspaceId string, subscriptionService subscription.Service, crossSpaceSubService crossspacesub.Service) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{
		mw:                   mw,
		gatewayUrl:           gatewayUrl,
		techSpaceId:          techspaceId,
		subscriptionService:  subscriptionService,
		crossSpaceSubService: crossSpaceSubService,
		componentCtx:         ctx,
		componentCtxCancel:   cancel,
		cache:                newCacheManager(),
		subscriptions:        newSubscriptions(),
	}

	return s
}
