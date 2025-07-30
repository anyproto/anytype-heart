package service

import (
	"sync"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("api-internal-service")

type Service struct {
	mw          apicore.ClientCommands
	gatewayUrl  string
	techSpaceId string

	subscriptionService subscription.Service
	subscriptionsMu     sync.RWMutex

	spaceSubscription     *objectsubscription.ObjectSubscription[string]                        // subscription for techspace to track space Ids
	typeSubscriptions     map[string]*objectsubscription.ObjectSubscription[*apimodel.Type]     // map[spaceId]*ObjectSubscription
	propertySubscriptions map[string]*objectsubscription.ObjectSubscription[*apimodel.Property] // map[spaceId]*ObjectSubscription
	tagSubscriptions      map[string]*objectsubscription.ObjectSubscription[*apimodel.Tag]      // map[spaceId]*ObjectSubscription

}

func NewService(mw apicore.ClientCommands, gatewayUrl string, techspaceId string, subscriptionService subscription.Service) *Service {
	s := &Service{
		mw:                    mw,
		gatewayUrl:            gatewayUrl,
		techSpaceId:           techspaceId,
		subscriptionService:   subscriptionService,
		typeSubscriptions:     make(map[string]*objectsubscription.ObjectSubscription[*apimodel.Type]),
		propertySubscriptions: make(map[string]*objectsubscription.ObjectSubscription[*apimodel.Property]),
		tagSubscriptions:      make(map[string]*objectsubscription.ObjectSubscription[*apimodel.Tag]),
	}

	return s
}

// getTypeMap builds a map of types from the subscription for quick lookups
func (s *Service) getTypeMap(spaceId string) map[string]*apimodel.Type {
	s.subscriptionsMu.RLock()
	sub := s.typeSubscriptions[spaceId]
	s.subscriptionsMu.RUnlock()

	typeMap := make(map[string]*apimodel.Type)
	if sub != nil {
		sub.Iterate(func(id string, t *apimodel.Type) bool {
			typeMap[t.Id] = t
			typeMap[t.Key] = t
			typeMap[t.UniqueKey] = t
			return true
		})
	}

	return typeMap
}

// getPropertyMap builds a map of properties from the subscription for quick lookups
func (s *Service) getPropertyMap(spaceId string) map[string]*apimodel.Property {
	s.subscriptionsMu.RLock()
	sub := s.propertySubscriptions[spaceId]
	s.subscriptionsMu.RUnlock()

	propertyMap := make(map[string]*apimodel.Property)
	if sub != nil {
		sub.Iterate(func(id string, prop *apimodel.Property) bool {
			propertyMap[prop.Id] = prop
			propertyMap[prop.Key] = prop
			propertyMap[prop.RelationKey] = prop
			return true
		})
	}

	return propertyMap
}

// getTagMap builds a map of tags from the subscription for quick lookups
func (s *Service) getTagMap(spaceId string) map[string]*apimodel.Tag {
	s.subscriptionsMu.RLock()
	sub := s.tagSubscriptions[spaceId]
	s.subscriptionsMu.RUnlock()

	tagMap := make(map[string]*apimodel.Tag)
	if sub != nil {
		sub.Iterate(func(id string, tag *apimodel.Tag) bool {
			tagMap[tag.Id] = tag
			return true
		})
	}

	return tagMap
}

// getAllSpaceIds returns all space IDs from the subscription
func (s *Service) getAllSpaceIds() []string {
	if s.spaceSubscription == nil {
		return nil
	}

	var spaceIds []string
	s.spaceSubscription.Iterate(func(id string, spaceId string) bool {
		if spaceId != "" {
			spaceIds = append(spaceIds, spaceId)
		}
		return true
	})

	return spaceIds
}
