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
	mw              apicore.ClientCommands
	subscriptionSvc subscription.Service
	gatewayUrl      string
	techSpaceId     string

	// Event queue for receiving updates from internal subscriptions
	eventQueue *mb.MB[*pb.EventMessage]

	// Cache maps for types, properties, and tags
	typeMapCache     map[string]map[string]*apimodel.Type     // map[spaceId]map[typeId]*Type
	propertyMapCache map[string]map[string]*apimodel.Property // map[spaceId]map[propertyId]*Property
	tagMapCache      map[string]map[string]*apimodel.Tag      // map[spaceId]map[tagId]*Tag

	// Subscription IDs for each space
	typeSubscriptions     map[string]string // map[spaceId]subscriptionId
	propertySubscriptions map[string]string // map[spaceId]subscriptionId
	tagSubscriptions      map[string]string // map[spaceId]subscriptionId
	spaceSubscriptionId   string            // subscription ID for space changes in tech space

	// Mutexes for thread-safe access
	typeMapMu     sync.RWMutex
	propertyMapMu sync.RWMutex
	tagMapMu      sync.RWMutex
}

func NewService(mw apicore.ClientCommands, gatewayUrl string, techspaceId string) *Service {
	s := &Service{
		mw:                    mw,
		gatewayUrl:            gatewayUrl,
		techSpaceId:           techspaceId,
		typeMapCache:          make(map[string]map[string]*apimodel.Type),
		propertyMapCache:      make(map[string]map[string]*apimodel.Property),
		tagMapCache:           make(map[string]map[string]*apimodel.Tag),
		typeSubscriptions:     make(map[string]string),
		propertySubscriptions: make(map[string]string),
		tagSubscriptions:      make(map[string]string),
	}

	return s
}

func (s *Service) SetSubscriptionService(svc subscription.Service) {
	s.subscriptionSvc = svc
}

func (s *Service) SetEventQueue(queue *mb.MB[*pb.EventMessage]) {
	s.eventQueue = queue
}

func (s *Service) getEventQueue() *mb.MB[*pb.EventMessage] {
	return s.eventQueue
}
