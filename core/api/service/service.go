package service

import (
	"context"
	"sync"

	"github.com/cheggaaa/mb/v3"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("api-internal-service")

type Service struct {
	mw          apicore.ClientCommands
	gatewayUrl  string
	techSpaceId string

	subscriptionService  subscription.Service
	crossSpaceSubService crossspacesub.Service
	subscriptionsMu      sync.RWMutex
	componentCtx         context.Context
	componentCtxCancel   context.CancelFunc

	// Cross-space subscription IDs
	propertySubId string
	typeSubId     string
	tagSubId      string

	// Event queues for cross-space subscriptions
	propertyQueue *mb.MB[*pb.EventMessage]
	typeQueue     *mb.MB[*pb.EventMessage]
	tagQueue      *mb.MB[*pb.EventMessage]

	// Caches organized by spaceId -> key -> object
	// For properties: key can be id, relationKey, or apiObjectKey
	// For types: key can be id, uniqueKey, or apiObjectKey
	// For tags: key is just id
	propertyCache map[string]map[string]*apimodel.Property // spaceId -> key -> Property
	typeCache     map[string]map[string]*apimodel.Type     // spaceId -> key -> Type
	tagCache      map[string]map[string]*apimodel.Tag      // spaceId -> id -> Tag
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
	}

	return s
}

// getTypeMap returns the type cache for a space for quick lookups
// The returned map should not be modified by callers
func (s *Service) getTypeMap(spaceId string) map[string]*apimodel.Type {
	s.subscriptionsMu.RLock()
	defer s.subscriptionsMu.RUnlock()

	if spaceCache, exists := s.typeCache[spaceId]; exists {
		return spaceCache
	}

	return make(map[string]*apimodel.Type)
}

// getPropertyMap returns the property cache for a space for quick lookups
// The returned map should not be modified by callers
func (s *Service) getPropertyMap(spaceId string) map[string]*apimodel.Property {
	s.subscriptionsMu.RLock()
	defer s.subscriptionsMu.RUnlock()

	if spaceCache, exists := s.propertyCache[spaceId]; exists {
		return spaceCache
	}

	return make(map[string]*apimodel.Property)
}

// getTagMap returns the tag cache for a space for quick lookups
// The returned map should not be modified by callers
func (s *Service) getTagMap(spaceId string) map[string]*apimodel.Tag {
	s.subscriptionsMu.RLock()
	defer s.subscriptionsMu.RUnlock()

	if spaceCache, exists := s.tagCache[spaceId]; exists {
		return spaceCache
	}

	return make(map[string]*apimodel.Tag)
}

// getAllSpaceIds returns all space IDs from the caches
func (s *Service) getAllSpaceIds() []string {
	s.subscriptionsMu.RLock()
	defer s.subscriptionsMu.RUnlock()

	spaceMap := make(map[string]bool)

	// Collect unique space IDs from all caches
	for spaceId := range s.propertyCache {
		spaceMap[spaceId] = true
	}
	for spaceId := range s.typeCache {
		spaceMap[spaceId] = true
	}
	for spaceId := range s.tagCache {
		spaceMap[spaceId] = true
	}

	var spaceIds []string
	for spaceId := range spaceMap {
		spaceIds = append(spaceIds, spaceId)
	}

	return spaceIds
}
