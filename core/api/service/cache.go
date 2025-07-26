package service

import (
	"context"
	"fmt"
	"strings"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var log = logging.Logger("anytype-api-service")

// InitializeAllCaches initializes caches for all available spaces
func (s *Service) InitializeAllCaches(ctx context.Context) error {
	if s.techSpaceId != "" {
		if err := s.subscribeToSpaceChanges(ctx); err != nil {
			return fmt.Errorf("failed to subscribe to space changes: %w", err)
		}
	}

	spaceIds, err := s.GetAllSpaceIds(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all space IDs: %w", err)
	}

	for _, spaceId := range spaceIds {
		if err := s.initializeSpaceCache(ctx, spaceId); err != nil {
			log.Debugf("failed to initialize cache for space %s: %v", spaceId, err)
		}
	}

	return nil
}

// subscribeToSpaceChanges subscribes to workspace/space changes in the tech space
func (s *Service) subscribeToSpaceChanges(ctx context.Context) error {
	if s.subscriptionSvc == nil {
		return fmt.Errorf("subscription service not available")
	}

	// Get the event queue from the parent API service
	queue := s.getEventQueue()
	if queue == nil {
		return fmt.Errorf("event queue not available")
	}

	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_spaceView)),
		},
	}

	resp, err := s.subscriptionSvc.Search(subscription.SubscribeRequest{
		SpaceId: s.techSpaceId,
		SubId:   "api_space_changes",
		Filters: filters,
		Keys: []string{
			bundle.RelationKeyTargetSpaceId.String(),
			bundle.RelationKeySpaceAccountStatus.String(),
			bundle.RelationKeySpaceLocalStatus.String(),
			bundle.RelationKeyIsArchived.String(),
			bundle.RelationKeyIsDeleted.String(),
		},
		Internal:      true,
		InternalQueue: queue,
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to space changes: %w", err)
	}

	s.spaceSubscriptionId = resp.SubId
	return nil
}

// initializeSpaceCache initializes all caches for a specific space
func (s *Service) initializeSpaceCache(ctx context.Context, spaceId string) error {
	if err := s.subscribeToProperties(ctx, spaceId); err != nil {
		return fmt.Errorf("failed to subscribe to properties: %w", err)
	}
	if err := s.subscribeToTypes(ctx, spaceId); err != nil {
		return fmt.Errorf("failed to subscribe to types: %w", err)
	}
	if err := s.subscribeToTags(ctx, spaceId); err != nil {
		return fmt.Errorf("failed to subscribe to tags: %w", err)
	}
	return nil
}

// subscribeToTypes subscribes to type changes for a space
func (s *Service) subscribeToTypes(ctx context.Context, spaceId string) error {
	s.typeMapMu.Lock()
	defer s.typeMapMu.Unlock()

	if _, exists := s.typeSubscriptions[spaceId]; exists {
		return nil
	}

	if s.subscriptionSvc == nil {
		return fmt.Errorf("subscription service not available")
	}

	// Get the event queue from the parent API service
	queue := s.getEventQueue()
	if queue == nil {
		return fmt.Errorf("event queue not available")
	}

	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_objectType)),
		},
		{
			RelationKey: bundle.RelationKeyIsArchived,
		},
	}

	resp, err := s.subscriptionSvc.Search(subscription.SubscribeRequest{
		SpaceId: spaceId,
		SubId:   fmt.Sprintf("api_types_%s", spaceId),
		Filters: filters,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyApiObjectKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyPluralName.String(),
			bundle.RelationKeyIconEmoji.String(),
			bundle.RelationKeyIconName.String(),
			bundle.RelationKeyIconOption.String(),
			bundle.RelationKeyRecommendedLayout.String(),
			bundle.RelationKeyIsArchived.String(),
			bundle.RelationKeyRecommendedFeaturedRelations.String(),
			bundle.RelationKeyRecommendedRelations.String(),
		},
		Internal:      true,
		InternalQueue: queue,
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to types: %w", err)
	}

	s.typeSubscriptions[spaceId] = resp.SubId

	if _, exists := s.typeMapCache[spaceId]; !exists {
		s.typeMapCache[spaceId] = make(map[string]*apimodel.Type)
	}

	s.propertyMapMu.RLock()
	propertyMap := s.propertyMapCache[spaceId]
	s.propertyMapMu.RUnlock()

	if propertyMap == nil {
		return fmt.Errorf("property cache not initialized for space %s", spaceId)
	}

	for _, record := range resp.Records {
		uk, apiKey, t := s.getTypeFromStruct(record.ToProto(), propertyMap)
		s.typeMapCache[spaceId][t.Id] = t
		s.typeMapCache[spaceId][apiKey] = t
		s.typeMapCache[spaceId][uk] = t
	}

	return nil
}

// subscribeToProperties subscribes to property changes for a space
func (s *Service) subscribeToProperties(ctx context.Context, spaceId string) error {
	s.propertyMapMu.Lock()
	defer s.propertyMapMu.Unlock()

	if _, exists := s.propertySubscriptions[spaceId]; exists {
		return nil
	}

	if s.subscriptionSvc == nil {
		return fmt.Errorf("subscription service not available")
	}

	// Get the event queue from the parent API service
	queue := s.getEventQueue()
	if queue == nil {
		return fmt.Errorf("event queue not available")
	}

	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_relation)),
		},
		{
			RelationKey: bundle.RelationKeyIsHidden,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		},
	}

	resp, err := s.subscriptionSvc.Search(subscription.SubscribeRequest{
		SpaceId: spaceId,
		SubId:   fmt.Sprintf("api_properties_%s", spaceId),
		Filters: filters,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyRelationKey.String(),
			bundle.RelationKeyApiObjectKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationFormat.String(),
		},
		Internal:      true,
		InternalQueue: queue,
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to properties: %w", err)
	}

	s.propertySubscriptions[spaceId] = resp.SubId

	if _, exists := s.propertyMapCache[spaceId]; !exists {
		s.propertyMapCache[spaceId] = make(map[string]*apimodel.Property)
	}

	for _, record := range resp.Records {
		rk, apiKey, prop := s.getPropertyFromStruct(record.ToProto())
		s.propertyMapCache[spaceId][prop.Id] = prop
		s.propertyMapCache[spaceId][rk] = prop
		s.propertyMapCache[spaceId][apiKey] = prop
	}

	return nil
}

// subscribeToTags subscribes to tag changes for a space
func (s *Service) subscribeToTags(ctx context.Context, spaceId string) error {
	s.tagMapMu.Lock()
	defer s.tagMapMu.Unlock()

	if _, exists := s.tagSubscriptions[spaceId]; exists {
		return nil
	}

	if s.subscriptionSvc == nil {
		return fmt.Errorf("subscription service not available")
	}

	// Get the event queue from the parent API service
	queue := s.getEventQueue()
	if queue == nil {
		return fmt.Errorf("event queue not available")
	}

	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_relationOption)),
		},
		{
			RelationKey: bundle.RelationKeyIsHidden,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		},
	}

	resp, err := s.subscriptionSvc.Search(subscription.SubscribeRequest{
		SpaceId: spaceId,
		SubId:   fmt.Sprintf("api_tags_%s", spaceId),
		Filters: filters,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		},
		Internal:      true,
		InternalQueue: queue,
	})

	if err != nil {
		return fmt.Errorf("failed to subscribe to tags: %w", err)
	}

	s.tagSubscriptions[spaceId] = resp.SubId

	if _, exists := s.tagMapCache[spaceId]; !exists {
		s.tagMapCache[spaceId] = make(map[string]*apimodel.Tag)
	}

	for _, record := range resp.Records {
		tag := s.getTagFromStruct(record.ToProto())
		s.tagMapCache[spaceId][tag.Id] = tag
	}

	return nil
}

// unsubscribeFromSpace unsubscribes from all subscriptions for a space
func (s *Service) unsubscribeFromSpace(ctx context.Context, spaceId string) {
	if s.subscriptionSvc == nil {
		return
	}

	s.typeMapMu.Lock()
	if subId, exists := s.typeSubscriptions[spaceId]; exists {
		s.subscriptionSvc.Unsubscribe(subId)
		delete(s.typeSubscriptions, spaceId)
		delete(s.typeMapCache, spaceId)
	}
	s.typeMapMu.Unlock()

	s.propertyMapMu.Lock()
	if subId, exists := s.propertySubscriptions[spaceId]; exists {
		s.subscriptionSvc.Unsubscribe(subId)
		delete(s.propertySubscriptions, spaceId)
		delete(s.propertyMapCache, spaceId)
	}
	s.propertyMapMu.Unlock()

	s.tagMapMu.Lock()
	if subId, exists := s.tagSubscriptions[spaceId]; exists {
		s.subscriptionSvc.Unsubscribe(subId)
		delete(s.tagSubscriptions, spaceId)
		delete(s.tagMapCache, spaceId)
	}
	s.tagMapMu.Unlock()
}

// ProcessSubscriptionEvent processes events from subscriptions to update caches in real-time
func (s *Service) ProcessSubscriptionEvent(event *pb.Event) {
	for _, msg := range event.Messages {
		switch v := msg.Value.(type) {
		case *pb.EventMessageValueOfObjectDetailsAmend:
			s.handleObjectDetailsAmend(v.ObjectDetailsAmend)
		case *pb.EventMessageValueOfObjectDetailsSet:
			s.handleObjectDetailsSet(v.ObjectDetailsSet)
		case *pb.EventMessageValueOfObjectRemove:
			s.handleObjectRemove(v.ObjectRemove)
		case *pb.EventMessageValueOfSubscriptionAdd:
			// New objects will be handled via ObjectDetailsSet events
		case *pb.EventMessageValueOfSubscriptionRemove:
			s.handleSubscriptionRemove(v.SubscriptionRemove)
		}
	}
}

// handleObjectDetailsAmend updates cache when object details are amended
func (s *Service) handleObjectDetailsAmend(amend *pb.EventObjectDetailsAmend) {
	// Check all subscription IDs this event applies to
	for _, subId := range amend.SubIds {
		if strings.HasPrefix(subId, "api_types_") {
			s.handleObjectUpdate(subId, amend.Id)
		} else if strings.HasPrefix(subId, "api_properties_") {
			s.handleObjectUpdate(subId, amend.Id)
		} else if strings.HasPrefix(subId, "api_tags_") {
			s.handleObjectUpdate(subId, amend.Id)
		}
	}
}

// handleObjectDetailsSet replaces entire object in cache
func (s *Service) handleObjectDetailsSet(set *pb.EventObjectDetailsSet) {
	// Check all subscription IDs this event applies to
	for _, subId := range set.SubIds {
		if strings.HasPrefix(subId, "api_types_") {
			s.handleObjectUpdate(subId, set.Id)
		} else if strings.HasPrefix(subId, "api_properties_") {
			s.handleObjectUpdate(subId, set.Id)
		} else if strings.HasPrefix(subId, "api_tags_") {
			s.handleObjectUpdate(subId, set.Id)
		}
	}
}

// handleObjectRemove removes object from cache
func (s *Service) handleObjectRemove(remove *pb.EventObjectRemove) {
	// Iterate through removed object IDs
	for _, objectId := range remove.Ids {
		// Check all cached spaces for this object
		s.typeMapMu.Lock()
		for _, cache := range s.typeMapCache {
			// Remove by all possible keys
			for k, v := range cache {
				if v.Id == objectId {
					delete(cache, k)
				}
			}
		}
		s.typeMapMu.Unlock()

		s.propertyMapMu.Lock()
		for _, cache := range s.propertyMapCache {
			for k, v := range cache {
				if v.Id == objectId {
					delete(cache, k)
				}
			}
		}
		s.propertyMapMu.Unlock()

		s.tagMapMu.Lock()
		for _, cache := range s.tagMapCache {
			delete(cache, objectId)
		}
		s.tagMapMu.Unlock()
	}
}

// handleSubscriptionRemove removes object from cache
func (s *Service) handleSubscriptionRemove(remove *pb.EventObjectSubscriptionRemove) {
	subId := remove.SubId
	if subId == "" {
		return
	}

	// Handle type cache
	if strings.HasPrefix(subId, "api_types_") {
		spaceId := strings.TrimPrefix(subId, "api_types_")
		s.typeMapMu.Lock()
		if cache, exists := s.typeMapCache[spaceId]; exists {
			for k, v := range cache {
				if v.Id == remove.Id {
					delete(cache, k)
				}
			}
		}
		s.typeMapMu.Unlock()
	}

	// Handle property cache
	if strings.HasPrefix(subId, "api_properties_") {
		spaceId := strings.TrimPrefix(subId, "api_properties_")
		s.propertyMapMu.Lock()
		if cache, exists := s.propertyMapCache[spaceId]; exists {
			for k, v := range cache {
				if v.Id == remove.Id {
					delete(cache, k)
				}
			}
		}
		s.propertyMapMu.Unlock()
	}

	// Handle tag cache
	if strings.HasPrefix(subId, "api_tags_") {
		spaceId := strings.TrimPrefix(subId, "api_tags_")
		s.tagMapMu.Lock()
		if cache, exists := s.tagMapCache[spaceId]; exists {
			delete(cache, remove.Id)
		}
		s.tagMapMu.Unlock()
	}
}

// Stop unsubscribes from all spaces and cleans up
func (s *Service) Stop() {
	if s.subscriptionSvc != nil {
		if s.spaceSubscriptionId != "" {
			s.subscriptionSvc.Unsubscribe(s.spaceSubscriptionId)
			s.spaceSubscriptionId = ""
		}

		s.typeMapMu.Lock()
		for _, subId := range s.typeSubscriptions {
			s.subscriptionSvc.Unsubscribe(subId)
		}
		s.typeSubscriptions = make(map[string]string)
		s.typeMapCache = make(map[string]map[string]*apimodel.Type)
		s.typeMapMu.Unlock()

		s.propertyMapMu.Lock()
		for _, subId := range s.propertySubscriptions {
			s.subscriptionSvc.Unsubscribe(subId)
		}
		s.propertySubscriptions = make(map[string]string)
		s.propertyMapCache = make(map[string]map[string]*apimodel.Property)
		s.propertyMapMu.Unlock()

		s.tagMapMu.Lock()
		for _, subId := range s.tagSubscriptions {
			s.subscriptionSvc.Unsubscribe(subId)
		}
		s.tagSubscriptions = make(map[string]string)
		s.tagMapCache = make(map[string]map[string]*apimodel.Tag)
		s.tagMapMu.Unlock()
	}
}

// handleObjectUpdate fetches fresh object data and updates cache
func (s *Service) handleObjectUpdate(subId string, objectId string) {
	ctx := context.Background()

	// Extract space ID from subscription ID
	var spaceId string
	if strings.HasPrefix(subId, "api_types_") {
		spaceId = strings.TrimPrefix(subId, "api_types_")

		// Fetch fresh object data
		resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
			SpaceId:  spaceId,
			ObjectId: objectId,
		})
		if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			log.Debugf("failed to fetch type object %s: %s", objectId, resp.Error)
			return
		}

		// Get property map for type conversion
		s.propertyMapMu.RLock()
		propertyMap := s.propertyMapCache[spaceId]
		s.propertyMapMu.RUnlock()

		if propertyMap != nil {
			if resp.ObjectView == nil || len(resp.ObjectView.Details) == 0 {
				log.Debugf("type object %s has no details", objectId)
				return
			}
			uk, apiKey, t := s.getTypeFromStruct(resp.ObjectView.Details[0].Details, propertyMap)

			s.typeMapMu.Lock()
			if cache, exists := s.typeMapCache[spaceId]; exists {
				cache[t.Id] = t
				cache[apiKey] = t
				cache[uk] = t
			}
			s.typeMapMu.Unlock()
		}
	} else if strings.HasPrefix(subId, "api_properties_") {
		spaceId = strings.TrimPrefix(subId, "api_properties_")

		// Fetch fresh object data
		resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
			SpaceId:  spaceId,
			ObjectId: objectId,
		})
		if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			log.Debugf("failed to fetch property object %s: %s", objectId, resp.Error)
			return
		}

		if resp.ObjectView == nil || len(resp.ObjectView.Details) == 0 {
			log.Debugf("property object %s has no details", objectId)
			return
		}
		rk, apiKey, prop := s.getPropertyFromStruct(resp.ObjectView.Details[0].Details)

		s.propertyMapMu.Lock()
		if cache, exists := s.propertyMapCache[spaceId]; exists {
			cache[prop.Id] = prop
			cache[rk] = prop
			cache[apiKey] = prop
		}
		s.propertyMapMu.Unlock()
	} else if strings.HasPrefix(subId, "api_tags_") {
		spaceId = strings.TrimPrefix(subId, "api_tags_")

		// Fetch fresh object data
		resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
			SpaceId:  spaceId,
			ObjectId: objectId,
		})
		if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			log.Debugf("failed to fetch tag object %s: %s", objectId, resp.Error)
			return
		}

		if resp.ObjectView == nil || len(resp.ObjectView.Details) == 0 {
			log.Debugf("tag object %s has no details", objectId)
			return
		}
		tag := s.getTagFromStruct(resp.ObjectView.Details[0].Details)

		s.tagMapMu.Lock()
		if cache, exists := s.tagMapCache[spaceId]; exists {
			cache[tag.Id] = tag
		}
		s.tagMapMu.Unlock()
	}
}
