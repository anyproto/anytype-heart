package service

import (
	"context"
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

func NewService(mw apicore.ClientCommands, gatewayUrl string, techspaceId string, subscriptionService subscription.Service) *Service {
	s := &Service{
		mw:                    mw,
		gatewayUrl:            gatewayUrl,
		techSpaceId:           techspaceId,
		subscriptionService:   subscriptionService,
		eventQueue:            mb.New[*pb.EventMessage](0),
		typeMapCache:          make(map[string]map[string]*apimodel.Type),
		propertyMapCache:      make(map[string]map[string]*apimodel.Property),
		tagMapCache:           make(map[string]map[string]*apimodel.Tag),
		typeSubscriptions:     make(map[string]string),
		propertySubscriptions: make(map[string]string),
		tagSubscriptions:      make(map[string]string),
	}

	// Start event processing goroutine
	go s.processEvents()

	return s
}

func (s *Service) processEvents() {
	for {
		msgs, err := s.eventQueue.Wait(context.Background())
		if err != nil {
			return
		}

		if len(msgs) > 0 {
			event := &pb.Event{Messages: msgs}
			s.ProcessSubscriptionEvent(event)
		}
	}
}
