package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
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

func (s *Service) InitializeAllCaches(ctx context.Context) error {
	if s.techSpaceId != "" {
		if err := s.subscribeToSpaceChanges(); err != nil {
			return fmt.Errorf("failed to subscribe to space changes: %w", err)
		}
	}

	spaceIds := s.getAllSpaceIds()
	for _, spaceId := range spaceIds {
		if err := s.initializeSpaceCache(spaceId); err != nil {
			log.Warnf("failed to initialize cache for space %s: %v", spaceId, err)
		}
	}

	return nil
}

func (s *Service) subscribeToSpaceChanges() error {
	if s.spaceSubscription != nil {
		return nil
	}

	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_spaceView)),
		},
		{
			RelationKey: bundle.RelationKeySpaceLocalStatus,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List([]int64{int64(model.SpaceStatus_Unknown), int64(model.SpaceStatus_Ok)}),
		},
		{
			RelationKey: bundle.RelationKeySpaceAccountStatus,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List([]int64{int64(model.SpaceStatus_Unknown), int64(model.SpaceStatus_SpaceActive)}),
		},
	}

	sub := objectsubscription.New(s.subscriptionService, objectsubscription.SubscriptionParams[string]{
		Request: subscription.SubscribeRequest{
			SpaceId: s.techSpaceId,
			SubId:   "api.space.changes",
			Filters: filters,
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyTargetSpaceId.String(),
				bundle.RelationKeySpaceAccountStatus.String(),
				bundle.RelationKeySpaceLocalStatus.String(),
				bundle.RelationKeyIsArchived.String(),
				bundle.RelationKeyIsDeleted.String(),
			},
			NoDepSubscription: true,
		},
		Extract: func(details *domain.Details) (string, string) {
			id := details.GetString(bundle.RelationKeyId)
			spaceId := details.GetString(bundle.RelationKeyTargetSpaceId)

			// Schedule cache initialization asynchronously to avoid deadlock
			// The subscription is holding a lock when calling Extract, so we can't
			// create new subscriptions synchronously here
			if spaceId != "" {
				go func() {
					if err := s.initializeSpaceCache(spaceId); err != nil {
						log.Warnf("failed to initialize cache for space %s: %v", spaceId, err)
					}
				}()
			}

			return id, spaceId
		},
		Update: func(key string, value domain.Value, spaceId string) string {
			return spaceId
		},
		Unset: func(keys []string, spaceId string) string {
			return spaceId
		},
		Remove: func(id string, spaceId string) string {
			// Space no longer matches filters - clean up its caches asynchronously
			// to avoid deadlock (Remove is called while subscription holds a lock)
			if spaceId != "" {
				go func() {
					s.unsubscribeFromSpace(spaceId)
				}()
			}
			return spaceId
		},
	})

	if err := sub.Run(); err != nil {
		return fmt.Errorf("failed to subscribe to space changes: %w", err)
	}

	s.spaceSubscription = sub
	return nil
}

// initializeSpaceCache initializes all caches for a specific space
func (s *Service) initializeSpaceCache(spaceId string) error {
	if err := s.subscribeToProperties(spaceId); err != nil {
		return fmt.Errorf("failed to subscribe to properties: %w", err)
	}
	if err := s.subscribeToTypes(spaceId); err != nil {
		return fmt.Errorf("failed to subscribe to types: %w", err)
	}
	if err := s.subscribeToTags(spaceId); err != nil {
		return fmt.Errorf("failed to subscribe to tags: %w", err)
	}
	return nil
}

// subscribeToTypes subscribes to type changes for a space
func (s *Service) subscribeToTypes(spaceId string) error {
	s.subscriptionsMu.Lock()
	defer s.subscriptionsMu.Unlock()

	if _, exists := s.typeSubscriptions[spaceId]; exists {
		return nil
	}

	propSub := s.propertySubscriptions[spaceId]
	if propSub == nil {
		return fmt.Errorf("property subscription not initialized for space %s", spaceId)
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

	sub := objectsubscription.New(s.subscriptionService, objectsubscription.SubscriptionParams[*apimodel.Type]{
		Request: subscription.SubscribeRequest{
			SpaceId: spaceId,
			SubId:   fmt.Sprintf("api.types.%s", spaceId),
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
			NoDepSubscription: true,
		},
		Extract: func(details *domain.Details) (string, *apimodel.Type) {
			propertyMap := make(map[string]*apimodel.Property)
			propSub.Iterate(func(id string, prop *apimodel.Property) bool {
				propertyMap[id] = prop
				propertyMap[prop.Key] = prop
				propertyMap[prop.RelationKey] = prop
				return true
			})

			_, _, t := s.getTypeFromStruct(details.ToProto(), propertyMap)
			return t.Id, t
		},
		Update: func(key string, value domain.Value, t *apimodel.Type) *apimodel.Type {
			details, err := s.getObjectDetails(context.Background(), spaceId, t.Id)
			if err != nil {
				return t
			}

			// Get property map for type construction
			propertyMap := make(map[string]*apimodel.Property)
			propSub.Iterate(func(id string, prop *apimodel.Property) bool {
				propertyMap[id] = prop
				propertyMap[prop.Key] = prop
				propertyMap[prop.RelationKey] = prop
				return true
			})

			_, _, updatedType := s.getTypeFromStruct(details, propertyMap)
			return updatedType
		},
		Unset: func(keys []string, t *apimodel.Type) *apimodel.Type {
			details, err := s.getObjectDetails(context.Background(), spaceId, t.Id)
			if err != nil {
				return t
			}

			// Get property map for type construction
			propertyMap := make(map[string]*apimodel.Property)
			propSub.Iterate(func(id string, prop *apimodel.Property) bool {
				propertyMap[id] = prop
				propertyMap[prop.Key] = prop
				propertyMap[prop.RelationKey] = prop
				return true
			})

			_, _, updatedType := s.getTypeFromStruct(details, propertyMap)
			return updatedType
		},
		Remove: func(id string, t *apimodel.Type) *apimodel.Type {
			return t
		},
	})

	if err := sub.Run(); err != nil {
		return fmt.Errorf("failed to subscribe to types: %w", err)
	}

	s.typeSubscriptions[spaceId] = sub
	return nil
}

// subscribeToProperties subscribes to property changes for a space
func (s *Service) subscribeToProperties(spaceId string) error {
	s.subscriptionsMu.Lock()
	defer s.subscriptionsMu.Unlock()

	if _, exists := s.propertySubscriptions[spaceId]; exists {
		return nil
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

	sub := objectsubscription.New(s.subscriptionService, objectsubscription.SubscriptionParams[*apimodel.Property]{
		Request: subscription.SubscribeRequest{
			SpaceId: spaceId,
			SubId:   fmt.Sprintf("api.properties.%s", spaceId),
			Filters: filters,
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyRelationKey.String(),
				bundle.RelationKeyApiObjectKey.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyRelationFormat.String(),
			},
			NoDepSubscription: true,
		},
		Extract: func(details *domain.Details) (string, *apimodel.Property) {
			_, _, prop := s.getPropertyFromStruct(details.ToProto())
			return prop.Id, prop
		},
		Update: func(key string, value domain.Value, prop *apimodel.Property) *apimodel.Property {
			details, err := s.getObjectDetails(context.Background(), spaceId, prop.Id)
			if err != nil {
				return prop
			}

			_, _, updatedProp := s.getPropertyFromStruct(details)
			return updatedProp
		},
		Unset: func(keys []string, prop *apimodel.Property) *apimodel.Property {
			details, err := s.getObjectDetails(context.Background(), spaceId, prop.Id)
			if err != nil {
				return prop
			}

			_, _, updatedProp := s.getPropertyFromStruct(details)
			return updatedProp
		},
		Remove: func(id string, prop *apimodel.Property) *apimodel.Property {
			return prop
		},
	})

	if err := sub.Run(); err != nil {
		return fmt.Errorf("failed to subscribe to properties: %w", err)
	}

	s.propertySubscriptions[spaceId] = sub
	return nil
}

// subscribeToTags subscribes to tag changes for a space
func (s *Service) subscribeToTags(spaceId string) error {
	s.subscriptionsMu.Lock()
	defer s.subscriptionsMu.Unlock()

	if _, exists := s.tagSubscriptions[spaceId]; exists {
		return nil
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

	sub := objectsubscription.New(s.subscriptionService, objectsubscription.SubscriptionParams[*apimodel.Tag]{
		Request: subscription.SubscribeRequest{
			SpaceId: spaceId,
			SubId:   fmt.Sprintf("api.tags.%s", spaceId),
			Filters: filters,
			Keys: []string{
				bundle.RelationKeyId.String(),
				bundle.RelationKeyUniqueKey.String(),
				bundle.RelationKeyName.String(),
				bundle.RelationKeyRelationOptionColor.String(),
			},
			NoDepSubscription: true,
		},
		Extract: func(details *domain.Details) (string, *apimodel.Tag) {
			tag := s.getTagFromStruct(details.ToProto())
			return tag.Id, tag
		},
		Update: func(key string, value domain.Value, tag *apimodel.Tag) *apimodel.Tag {
			details, err := s.getObjectDetails(context.Background(), spaceId, tag.Id)
			if err != nil {
				return tag
			}

			updatedTag := s.getTagFromStruct(details)
			return updatedTag
		},
		Unset: func(keys []string, tag *apimodel.Tag) *apimodel.Tag {
			details, err := s.getObjectDetails(context.Background(), spaceId, tag.Id)
			if err != nil {
				return tag
			}

			updatedTag := s.getTagFromStruct(details)
			return updatedTag
		},
		Remove: func(id string, tag *apimodel.Tag) *apimodel.Tag {
			return tag
		},
	})

	if err := sub.Run(); err != nil {
		return fmt.Errorf("failed to subscribe to tags: %w", err)
	}

	s.tagSubscriptions[spaceId] = sub
	return nil
}

// unsubscribeFromSpace unsubscribes from all subscriptions for a space
func (s *Service) unsubscribeFromSpace(spaceId string) {
	s.subscriptionsMu.Lock()
	defer s.subscriptionsMu.Unlock()

	if sub, exists := s.typeSubscriptions[spaceId]; exists {
		sub.Close()
		delete(s.typeSubscriptions, spaceId)
	}

	if sub, exists := s.propertySubscriptions[spaceId]; exists {
		sub.Close()
		delete(s.propertySubscriptions, spaceId)
	}

	if sub, exists := s.tagSubscriptions[spaceId]; exists {
		sub.Close()
		delete(s.tagSubscriptions, spaceId)
	}
}

// Stop unsubscribes from all spaces and cleans up
func (s *Service) Stop() {
	if s.spaceSubscription != nil {
		s.spaceSubscription.Close()
		s.spaceSubscription = nil
	}

	s.subscriptionsMu.Lock()
	defer s.subscriptionsMu.Unlock()

	for _, sub := range s.typeSubscriptions {
		sub.Close()
	}
	s.typeSubscriptions = nil

	for _, sub := range s.propertySubscriptions {
		sub.Close()
	}
	s.propertySubscriptions = nil

	for _, sub := range s.tagSubscriptions {
		sub.Close()
	}
	s.tagSubscriptions = nil
}
