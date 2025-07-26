package service

import (
	"sync"

	"github.com/cheggaaa/mb/v3"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
)

type Service struct {
	mw          apicore.ClientCommands
	gatewayUrl  string
	techSpaceId string

	subscriptionService subscription.Service
	eventQueue          *mb.MB[*pb.EventMessage]

	typeMapCache     map[string]map[string]*apimodel.Type     // map[spaceId]map[typeId]*Type
	propertyMapCache map[string]map[string]*apimodel.Property // map[spaceId]map[propertyId]*Property
	tagMapCache      map[string]map[string]*apimodel.Tag      // map[spaceId]map[tagId]*Tag

	spaceSubscriptionId   string            // space changes in tech space
	typeSubscriptions     map[string]string // map[spaceId]subscriptionId
	propertySubscriptions map[string]string // map[spaceId]subscriptionId
	tagSubscriptions      map[string]string // map[spaceId]subscriptionId

	typeMapMu     sync.RWMutex
	propertyMapMu sync.RWMutex
	tagMapMu      sync.RWMutex
}

func NewService(mw apicore.ClientCommands, gatewayUrl string, techspaceId string, subscriptionService subscription.Service, eventQueue *mb.MB[*pb.EventMessage]) *Service {
	s := &Service{
		mw:                    mw,
		gatewayUrl:            gatewayUrl,
		techSpaceId:           techspaceId,
		subscriptionService:   subscriptionService,
		eventQueue:            eventQueue,
		typeMapCache:          make(map[string]map[string]*apimodel.Type),
		propertyMapCache:      make(map[string]map[string]*apimodel.Property),
		tagMapCache:           make(map[string]map[string]*apimodel.Tag),
		typeSubscriptions:     make(map[string]string),
		propertySubscriptions: make(map[string]string),
		tagSubscriptions:      make(map[string]string),
	}

	return s
}
