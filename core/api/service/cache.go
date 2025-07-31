package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// getObjectDetails fetches object details by Id using ObjectSearch
func (s *Service) getObjectDetails(ctx context.Context, spaceId, objectId string) (*types.Struct, error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(objectId),
			},
		},
		Limit: 1,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, fmt.Errorf("failed to fetch object: %w", errors.New(resp.Error.Description))
	}

	if len(resp.Records) == 0 {
		return nil, fmt.Errorf("object not found: %s", objectId)
	}

	return resp.Records[0], nil
}

func (s *Service) InitializeAllCaches() error {
	// Initialize the cross-space subscriptions for types, properties, and tags
	if err := s.subscribeToCrossSpaceProperties(); err != nil {
		return fmt.Errorf("failed to subscribe to cross-space properties: %w", err)
	}

	if err := s.subscribeToCrossSpaceTypes(); err != nil {
		return fmt.Errorf("failed to subscribe to cross-space types: %w", err)
	}

	if err := s.subscribeToCrossSpaceTags(); err != nil {
		return fmt.Errorf("failed to subscribe to cross-space tags: %w", err)
	}

	return nil
}

// subscribeToCrossSpaceProperties subscribes to property changes across all active spaces
func (s *Service) subscribeToCrossSpaceProperties() error {
	// No lock needed during initialization - protected by sync.Once in middleware
	if s.propertyQueue != nil {
		return nil // Already subscribed
	}

	s.propertyQueue = mb.New[*pb.EventMessage](0)
	s.propertyCache = make(map[string]map[string]*apimodel.Property) // spaceId -> key -> Property

	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_relation)),
		},
	}

	resp, err := s.crossSpaceSubService.Subscribe(subscription.SubscribeRequest{
		SubId:   "api.properties.crossspace",
		Filters: filters,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyRelationKey.String(),
			bundle.RelationKeyApiObjectKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationFormat.String(),
			bundle.RelationKeySpaceId.String(),
		},
		NoDepSubscription: true,
		InternalQueue:     s.propertyQueue,
	}, crossspacesub.NoOpPredicate())

	if err != nil {
		return fmt.Errorf("failed to subscribe to cross-space properties: %w", err)
	}

	s.propertySubId = resp.SubId

	// Process initial records - no concurrent access during initialization
	for _, record := range resp.Records {
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if spaceId == "" {
			continue
		}
		_, _, prop := s.getPropertyFromStruct(record.ToProto())
		s.cacheProperty(spaceId, prop)
	}

	// Start processing property events
	go s.processPropertyEvents()

	return nil
}

// subscribeToCrossSpaceTypes subscribes to type changes across all active spaces
func (s *Service) subscribeToCrossSpaceTypes() error {
	// No lock needed during initialization - protected by sync.Once in middleware
	if s.typeQueue != nil {
		return nil // Already subscribed
	}

	s.typeQueue = mb.New[*pb.EventMessage](0)
	s.typeCache = make(map[string]map[string]*apimodel.Type) // spaceId -> key -> Type

	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_objectType)),
		},
	}

	resp, err := s.crossSpaceSubService.Subscribe(subscription.SubscribeRequest{
		SubId:   "api.types.crossspace",
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
			bundle.RelationKeySpaceId.String(),
		},
		NoDepSubscription: true,
		InternalQueue:     s.typeQueue,
	}, crossspacesub.NoOpPredicate())

	if err != nil {
		return fmt.Errorf("failed to subscribe to cross-space types: %w", err)
	}

	s.typeSubId = resp.SubId

	// Process initial records - no concurrent access during initialization
	for _, record := range resp.Records {
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if spaceId == "" {
			continue
		}
		// Get property map for this space
		propertyMap := s.getPropertyMapForSpace(spaceId)
		_, _, t := s.getTypeFromStruct(record.ToProto(), propertyMap)
		s.cacheType(spaceId, t)
	}

	// Start processing type events
	go s.processTypeEvents()

	return nil
}

// subscribeToCrossSpaceTags subscribes to tag changes across all active spaces
func (s *Service) subscribeToCrossSpaceTags() error {
	// No lock needed during initialization - protected by sync.Once in middleware
	if s.tagQueue != nil {
		return nil // Already subscribed
	}

	s.tagQueue = mb.New[*pb.EventMessage](0)
	s.tagCache = make(map[string]map[string]*apimodel.Tag) // spaceId -> id -> Tag

	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_relationOption)),
		},
	}

	resp, err := s.crossSpaceSubService.Subscribe(subscription.SubscribeRequest{
		SubId:   "api.tags.crossspace",
		Filters: filters,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationOptionColor.String(),
			bundle.RelationKeySpaceId.String(),
		},
		NoDepSubscription: true,
		InternalQueue:     s.tagQueue,
	}, crossspacesub.NoOpPredicate())

	if err != nil {
		return fmt.Errorf("failed to subscribe to cross-space tags: %w", err)
	}

	s.tagSubId = resp.SubId

	// Process initial records - no concurrent access during initialization
	for _, record := range resp.Records {
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if spaceId == "" {
			continue
		}
		tag := s.getTagFromStruct(record.ToProto())
		s.cacheTag(spaceId, tag)
	}

	// Start processing tag events
	go s.processTagEvents()

	return nil
}

// processPropertyEvents processes property change events from the queue
func (s *Service) processPropertyEvents() {
	for {
		messages, err := s.propertyQueue.Wait(s.componentCtx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Errorf("error waiting for property messages: %v", err)
			continue
		}

		for _, msg := range messages {
			s.handlePropertyEvent(msg)
		}
	}
}

// processTypeEvents processes type change events from the queue
func (s *Service) processTypeEvents() {
	for {
		messages, err := s.typeQueue.Wait(s.componentCtx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Errorf("error waiting for type messages: %v", err)
			continue
		}

		for _, msg := range messages {
			s.handleTypeEvent(msg)
		}
	}
}

// processTagEvents processes tag change events from the queue
func (s *Service) processTagEvents() {
	for {
		messages, err := s.tagQueue.Wait(s.componentCtx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			log.Errorf("error waiting for tag messages: %v", err)
			continue
		}

		for _, msg := range messages {
			s.handleTagEvent(msg)
		}
	}
}

// handlePropertyEvent handles a single property event
func (s *Service) handlePropertyEvent(msg *pb.EventMessage) {
	spaceId := msg.SpaceId

	switch value := msg.Value.(type) {
	case *pb.EventMessageValueOfSubscriptionAdd:
		// For cross-space subscriptions, we need to fetch the full details
		details, err := s.getObjectDetails(context.Background(), spaceId, value.SubscriptionAdd.Id)
		if err != nil {
			log.Errorf("failed to get property details for %s: %v", value.SubscriptionAdd.Id, err)
			return
		}
		_, _, prop := s.getPropertyFromStruct(details)
		s.subscriptionsMu.Lock()
		s.cacheProperty(spaceId, prop)
		s.subscriptionsMu.Unlock()

	case *pb.EventMessageValueOfSubscriptionRemove:
		s.subscriptionsMu.Lock()
		if spaceCache, exists := s.propertyCache[spaceId]; exists {
			if prop, exists := spaceCache[value.SubscriptionRemove.Id]; exists {
				// Remove all keys pointing to this property
				delete(spaceCache, prop.Id)
				delete(spaceCache, prop.RelationKey)
				delete(spaceCache, prop.Key)
			}
		}
		s.subscriptionsMu.Unlock()

	case *pb.EventMessageValueOfObjectDetailsSet, *pb.EventMessageValueOfObjectDetailsUnset, *pb.EventMessageValueOfObjectDetailsAmend:
		// Re-fetch the property details
		if spaceCache, exists := s.propertyCache[spaceId]; exists {
			for _, prop := range spaceCache {
				details, err := s.getObjectDetails(context.Background(), spaceId, prop.Id)
				if err == nil {
					_, _, updatedProp := s.getPropertyFromStruct(details)
					s.subscriptionsMu.Lock()
					s.cacheProperty(spaceId, updatedProp)
					s.subscriptionsMu.Unlock()
				}
			}
		}
	}
}

// handleTypeEvent handles a single type event
func (s *Service) handleTypeEvent(msg *pb.EventMessage) {
	spaceId := msg.SpaceId

	switch value := msg.Value.(type) {
	case *pb.EventMessageValueOfSubscriptionAdd:
		// For cross-space subscriptions, we need to fetch the full details
		details, err := s.getObjectDetails(context.Background(), spaceId, value.SubscriptionAdd.Id)
		if err != nil {
			log.Errorf("failed to get type details for %s: %v", value.SubscriptionAdd.Id, err)
			return
		}
		propertyMap := s.getPropertyMapForSpace(spaceId)
		_, _, t := s.getTypeFromStruct(details, propertyMap)
		s.subscriptionsMu.Lock()
		s.cacheType(spaceId, t)
		s.subscriptionsMu.Unlock()

	case *pb.EventMessageValueOfSubscriptionRemove:
		s.subscriptionsMu.Lock()
		if spaceCache, exists := s.typeCache[spaceId]; exists {
			if t, exists := spaceCache[value.SubscriptionRemove.Id]; exists {
				// Remove all keys pointing to this type
				delete(spaceCache, t.Id)
				delete(spaceCache, t.UniqueKey)
				delete(spaceCache, t.Key)
			}
		}
		s.subscriptionsMu.Unlock()

	case *pb.EventMessageValueOfObjectDetailsSet, *pb.EventMessageValueOfObjectDetailsUnset, *pb.EventMessageValueOfObjectDetailsAmend:
		// Re-fetch the type details
		if spaceCache, exists := s.typeCache[spaceId]; exists {
			for _, t := range spaceCache {
				details, err := s.getObjectDetails(context.Background(), spaceId, t.Id)
				if err == nil {
					propertyMap := s.getPropertyMapForSpace(spaceId)
					_, _, updatedType := s.getTypeFromStruct(details, propertyMap)
					s.subscriptionsMu.Lock()
					s.cacheType(spaceId, updatedType)
					s.subscriptionsMu.Unlock()
				}
			}
		}
	}
}

// handleTagEvent handles a single tag event
func (s *Service) handleTagEvent(msg *pb.EventMessage) {
	spaceId := msg.SpaceId

	switch value := msg.Value.(type) {
	case *pb.EventMessageValueOfSubscriptionAdd:
		// For cross-space subscriptions, we need to fetch the full details
		details, err := s.getObjectDetails(context.Background(), spaceId, value.SubscriptionAdd.Id)
		if err != nil {
			log.Errorf("failed to get tag details for %s: %v", value.SubscriptionAdd.Id, err)
			return
		}
		tag := s.getTagFromStruct(details)
		s.subscriptionsMu.Lock()
		s.cacheTag(spaceId, tag)
		s.subscriptionsMu.Unlock()

	case *pb.EventMessageValueOfSubscriptionRemove:
		s.subscriptionsMu.Lock()
		if spaceCache, exists := s.tagCache[spaceId]; exists {
			delete(spaceCache, value.SubscriptionRemove.Id)
		}
		s.subscriptionsMu.Unlock()

	case *pb.EventMessageValueOfObjectDetailsSet, *pb.EventMessageValueOfObjectDetailsUnset, *pb.EventMessageValueOfObjectDetailsAmend:
		// Re-fetch the tag details
		if spaceCache, exists := s.tagCache[spaceId]; exists {
			for _, tag := range spaceCache {
				details, err := s.getObjectDetails(context.Background(), spaceId, tag.Id)
				if err == nil {
					updatedTag := s.getTagFromStruct(details)
					s.subscriptionsMu.Lock()
					s.cacheTag(spaceId, updatedTag)
					s.subscriptionsMu.Unlock()
				}
			}
		}
	}
}

// Helper methods for caching
func (s *Service) cacheProperty(spaceId string, prop *apimodel.Property) {
	if _, exists := s.propertyCache[spaceId]; !exists {
		s.propertyCache[spaceId] = make(map[string]*apimodel.Property)
	}

	// Store with all possible keys: id, relationKey, apiObjectKey
	s.propertyCache[spaceId][prop.Id] = prop
	s.propertyCache[spaceId][prop.RelationKey] = prop
	s.propertyCache[spaceId][prop.Key] = prop
}

func (s *Service) cacheType(spaceId string, t *apimodel.Type) {
	if _, exists := s.typeCache[spaceId]; !exists {
		s.typeCache[spaceId] = make(map[string]*apimodel.Type)
	}

	// Store with all possible keys: id, uniqueKey, apiObjectKey
	s.typeCache[spaceId][t.Id] = t
	s.typeCache[spaceId][t.UniqueKey] = t
	s.typeCache[spaceId][t.Key] = t
}

func (s *Service) cacheTag(spaceId string, tag *apimodel.Tag) {
	if _, exists := s.tagCache[spaceId]; !exists {
		s.tagCache[spaceId] = make(map[string]*apimodel.Tag)
	}

	s.tagCache[spaceId][tag.Id] = tag
}

// getPropertyMapForSpace returns the property map for a specific space
func (s *Service) getPropertyMapForSpace(spaceId string) map[string]*apimodel.Property {
	s.subscriptionsMu.RLock()
	defer s.subscriptionsMu.RUnlock()

	// Return the cache directly since it already contains all keys
	if spaceCache, exists := s.propertyCache[spaceId]; exists {
		// Create a copy to avoid external modifications
		propertyMap := make(map[string]*apimodel.Property, len(spaceCache))
		for k, v := range spaceCache {
			propertyMap[k] = v
		}
		return propertyMap
	}
	return make(map[string]*apimodel.Property)
}

// Stop unsubscribes from all cross-space subscriptions and cleans up
func (s *Service) Stop() {
	if s.componentCtxCancel != nil {
		s.componentCtxCancel()
	}

	s.subscriptionsMu.Lock()
	defer s.subscriptionsMu.Unlock()

	if s.propertySubId != "" {
		if err := s.crossSpaceSubService.Unsubscribe(s.propertySubId); err != nil {
			log.Errorf("Failed to unsubscribe from cross-space properties: %v", err)
		}
	}

	if s.typeSubId != "" {
		if err := s.crossSpaceSubService.Unsubscribe(s.typeSubId); err != nil {
			log.Errorf("Failed to unsubscribe from cross-space types: %v", err)
		}
	}

	if s.tagSubId != "" {
		if err := s.crossSpaceSubService.Unsubscribe(s.tagSubId); err != nil {
			log.Errorf("Failed to unsubscribe from cross-space tags: %v", err)
		}
	}

	// Close queues
	if s.propertyQueue != nil {
		s.propertyQueue.Close()
	}
	if s.typeQueue != nil {
		s.typeQueue.Close()
	}
	if s.tagQueue != nil {
		s.tagQueue.Close()
	}

	// Clear caches
	s.propertyCache = nil
	s.typeCache = nil
	s.tagCache = nil
}
