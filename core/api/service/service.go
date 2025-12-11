package service

import (
	"context"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("api-internal-service")

type Service struct {
	mw          apicore.ClientCommands
	gatewayUrl  string
	techSpaceId string

	crossSpaceSubService apicore.CrossSpaceSubscriptionService
	componentCtxCancel   context.CancelFunc

	cache         *cacheManager
	subscriptions *subscriptions
}

func NewService(mw apicore.ClientCommands, gatewayUrl string, techspaceId string, crossSpaceSubService apicore.CrossSpaceSubscriptionService) *Service {
	_, cancel := context.WithCancel(context.Background())
	s := &Service{
		mw:                   mw,
		gatewayUrl:           gatewayUrl,
		techSpaceId:          techspaceId,
		crossSpaceSubService: crossSpaceSubService,
		componentCtxCancel:   cancel,
		cache:                newCacheManager(),
		subscriptions:        newSubscriptions(),
	}

	return s
}
