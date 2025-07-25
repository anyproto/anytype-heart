package service

import (
	"context"
	"fmt"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("anytype-api-service")

// InitializeAllCaches initializes caches for all available spaces
func (s *Service) InitializeAllCaches(ctx context.Context) error {
	// Subscribe to workspace/space changes in tech space
	if s.techSpaceId != "" {
		if err := s.subscribeToSpaceChanges(ctx); err != nil {
			return fmt.Errorf("failed to subscribe to space changes: %w", err)
		}
	}

	// Get all space IDs using the existing method
	spaceIds, err := s.GetAllSpaceIds(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all space IDs: %w", err)
	}

	// Initialize cache for each space (excluding tech space)
	for _, spaceId := range spaceIds {
		if err := s.initializeSpaceCache(ctx, spaceId); err != nil {
			// Log error but continue with other spaces
			log.Debugf("failed to initialize cache for space %s: %v", spaceId, err)
		}
	}

	return nil
}

// subscribeToSpaceChanges subscribes to workspace/space changes in the tech space
func (s *Service) subscribeToSpaceChanges(ctx context.Context) error {
	// Subscribe to workspace views in tech space
	resp := s.mw.ObjectSearchSubscribe(ctx, &pb.RpcObjectSearchSubscribeRequest{
		SpaceId: s.techSpaceId,
		SubId:   "api_space_changes",
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_spaceView)),
			},
		},
		Keys: []string{
			bundle.RelationKeyTargetSpaceId.String(),
			bundle.RelationKeySpaceAccountStatus.String(),
			bundle.RelationKeySpaceLocalStatus.String(),
			bundle.RelationKeyIsArchived.String(),
			bundle.RelationKeyIsDeleted.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchSubscribeResponseError_NULL {
		return fmt.Errorf("failed to subscribe to space changes: %s", resp.Error)
	}

	// Store subscription ID
	s.spaceSubscriptionId = resp.SubId

	// TODO: Handle subscription updates via event processing
	// Events will include:
	// - New workspaces being created (new records added)
	// - Workspaces being deleted or archived
	// - Workspace status changes
	// When a new space is created, call s.initializeSpaceCache(ctx, newSpaceId)
	// When a space is deleted/archived, call s.clearSpaceCache(spaceId)

	return nil
}

// initializeSpaceCache initializes all caches for a specific space
func (s *Service) initializeSpaceCache(ctx context.Context, spaceId string) error {
	// Subscribe to properties first as types depend on property map
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

	// Check if already subscribed
	if _, exists := s.typeSubscriptions[spaceId]; exists {
		return nil
	}

	// Subscribe to types
	resp := s.mw.ObjectSearchSubscribe(ctx, &pb.RpcObjectSearchSubscribeRequest{
		SpaceId: spaceId,
		SubId:   fmt.Sprintf("api_types_%s", spaceId),
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
			{
				// Include archived types as well
				RelationKey: bundle.RelationKeyIsArchived.String(),
			},
		},
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
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchSubscribeResponseError_NULL {
		return fmt.Errorf("failed to subscribe to types: %w", ErrFailedRetrieveTypes)
	}

	// Store subscription ID
	s.typeSubscriptions[spaceId] = resp.SubId

	// Initialize type map for this space if not exists
	if _, exists := s.typeMapCache[spaceId]; !exists {
		s.typeMapCache[spaceId] = make(map[string]*apimodel.Type)
	}

	// Get property map from cache for converting types
	s.propertyMapMu.RLock()
	propertyMap := s.propertyMapCache[spaceId]
	s.propertyMapMu.RUnlock()

	if propertyMap == nil {
		// Properties should have been loaded first
		return fmt.Errorf("property cache not initialized for space %s", spaceId)
	}

	// Process initial records
	for _, record := range resp.Records {
		uk, apiKey, t := s.getTypeFromStruct(record, propertyMap)
		s.typeMapCache[spaceId][t.Id] = t
		s.typeMapCache[spaceId][apiKey] = t
		s.typeMapCache[spaceId][uk] = t
	}

	// TODO: Handle subscription updates via event processing
	// Updates are delivered through EventService.Broadcast() with event types like:
	// - EventModel_ObjectDetailsAmend for updates
	// - EventModel_ObjectRemove for deletions
	// The events contain subscription ID which can be matched against our stored subscription IDs

	return nil
}

// subscribeToProperties subscribes to property changes for a space
func (s *Service) subscribeToProperties(ctx context.Context, spaceId string) error {
	s.propertyMapMu.Lock()
	defer s.propertyMapMu.Unlock()

	// Check if already subscribed
	if _, exists := s.propertySubscriptions[spaceId]; exists {
		return nil
	}

	// Subscribe to properties
	resp := s.mw.ObjectSearchSubscribe(ctx, &pb.RpcObjectSearchSubscribeRequest{
		SpaceId: spaceId,
		SubId:   fmt.Sprintf("api_properties_%s", spaceId),
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Bool(true),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyRelationKey.String(),
			bundle.RelationKeyApiObjectKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationFormat.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchSubscribeResponseError_NULL {
		return fmt.Errorf("failed to subscribe to properties: %w", ErrFailedRetrievePropertyMap)
	}

	// Store subscription ID
	s.propertySubscriptions[spaceId] = resp.SubId

	// Initialize property map for this space if not exists
	if _, exists := s.propertyMapCache[spaceId]; !exists {
		s.propertyMapCache[spaceId] = make(map[string]*apimodel.Property)
	}

	// Process initial records
	for _, record := range resp.Records {
		rk, apiKey, prop := s.getPropertyFromStruct(record)
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

	// Check if already subscribed
	if _, exists := s.tagSubscriptions[spaceId]; exists {
		return nil
	}

	// Subscribe to tags
	resp := s.mw.ObjectSearchSubscribe(ctx, &pb.RpcObjectSearchSubscribeRequest{
		SpaceId: spaceId,
		SubId:   fmt.Sprintf("api_tags_%s", spaceId),
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relationOption)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Bool(true),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationOptionColor.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchSubscribeResponseError_NULL {
		return fmt.Errorf("failed to subscribe to tags: %w", ErrFailedRetrieveTags)
	}

	// Store subscription ID
	s.tagSubscriptions[spaceId] = resp.SubId

	// Initialize tag map for this space if not exists
	if _, exists := s.tagMapCache[spaceId]; !exists {
		s.tagMapCache[spaceId] = make(map[string]*apimodel.Tag)
	}

	// Process initial records
	for _, record := range resp.Records {
		tag := s.getTagFromStruct(record)
		s.tagMapCache[spaceId][tag.Id] = tag
	}

	return nil
}

// unsubscribeFromSpace unsubscribes from all subscriptions for a space
func (s *Service) unsubscribeFromSpace(ctx context.Context, spaceId string) {
	// Unsubscribe from types
	s.typeMapMu.Lock()
	if subId, exists := s.typeSubscriptions[spaceId]; exists {
		s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
			SubIds: []string{subId},
		})
		delete(s.typeSubscriptions, spaceId)
		delete(s.typeMapCache, spaceId)
	}
	s.typeMapMu.Unlock()

	// Unsubscribe from properties
	s.propertyMapMu.Lock()
	if subId, exists := s.propertySubscriptions[spaceId]; exists {
		s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
			SubIds: []string{subId},
		})
		delete(s.propertySubscriptions, spaceId)
		delete(s.propertyMapCache, spaceId)
	}
	s.propertyMapMu.Unlock()

	// Unsubscribe from tags
	s.tagMapMu.Lock()
	if subId, exists := s.tagSubscriptions[spaceId]; exists {
		s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
			SubIds: []string{subId},
		})
		delete(s.tagSubscriptions, spaceId)
		delete(s.tagMapCache, spaceId)
	}
	s.tagMapMu.Unlock()
}

// Stop unsubscribes from all spaces and cleans up
func (s *Service) Stop() {
	ctx := context.Background()

	// Unsubscribe from space changes in tech space
	if s.spaceSubscriptionId != "" {
		s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
			SubIds: []string{s.spaceSubscriptionId},
		})
		s.spaceSubscriptionId = ""
	}

	// Unsubscribe from all type subscriptions
	s.typeMapMu.Lock()
	for _, subId := range s.typeSubscriptions {
		s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
			SubIds: []string{subId},
		})
	}
	s.typeSubscriptions = make(map[string]string)
	s.typeMapCache = make(map[string]map[string]*apimodel.Type)
	s.typeMapMu.Unlock()

	// Unsubscribe from all property subscriptions
	s.propertyMapMu.Lock()
	for _, subId := range s.propertySubscriptions {
		s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
			SubIds: []string{subId},
		})
	}
	s.propertySubscriptions = make(map[string]string)
	s.propertyMapCache = make(map[string]map[string]*apimodel.Property)
	s.propertyMapMu.Unlock()

	// Unsubscribe from all tag subscriptions
	s.tagMapMu.Lock()
	for _, subId := range s.tagSubscriptions {
		s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
			SubIds: []string{subId},
		})
	}
	s.tagSubscriptions = make(map[string]string)
	s.tagMapCache = make(map[string]map[string]*apimodel.Tag)
	s.tagMapMu.Unlock()
}

// Cache invalidation methods - now they just clear the cache and re-subscribe

func (s *Service) invalidateTypeCache(spaceId string) {
	ctx := context.Background()
	s.typeMapMu.Lock()
	if subId, exists := s.typeSubscriptions[spaceId]; exists {
		s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
			SubIds: []string{subId},
		})
		delete(s.typeSubscriptions, spaceId)
		delete(s.typeMapCache, spaceId)
	}
	s.typeMapMu.Unlock()
}

func (s *Service) invalidatePropertyCache(spaceId string) {
	ctx := context.Background()
	s.propertyMapMu.Lock()
	if subId, exists := s.propertySubscriptions[spaceId]; exists {
		s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
			SubIds: []string{subId},
		})
		delete(s.propertySubscriptions, spaceId)
		delete(s.propertyMapCache, spaceId)
	}
	s.propertyMapMu.Unlock()
}

func (s *Service) invalidateTagCache(spaceId string) {
	ctx := context.Background()
	s.tagMapMu.Lock()
	if subId, exists := s.tagSubscriptions[spaceId]; exists {
		s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
			SubIds: []string{subId},
		})
		delete(s.tagSubscriptions, spaceId)
		delete(s.tagMapCache, spaceId)
	}
	s.tagMapMu.Unlock()
}

func (s *Service) clearSpaceCache(spaceId string) {
	s.invalidateTypeCache(spaceId)
	s.invalidatePropertyCache(spaceId)
	s.invalidateTagCache(spaceId)
}
