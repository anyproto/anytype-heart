package service

import (
	"context"
	"fmt"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *Service) InitializeAllCaches(ctx context.Context) error {
	if s.techSpaceId != "" {
		if err := s.subscribeToSpaceChanges(); err != nil {
			return fmt.Errorf("failed to subscribe to space changes: %w", err)
		}
	}

	spaceIds, err := s.GetAllSpaceIds(ctx)
	if err != nil {
		return fmt.Errorf("failed to get all space Ids: %w", err)
	}

	for _, spaceId := range spaceIds {
		if err := s.initializeSpaceCache(spaceId); err != nil {
			log.Debugf("failed to initialize cache for space %s: %v", spaceId, err)
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
	}

	sub := objectsubscription.New(s.subscriptionService, objectsubscription.SubscriptionParams[struct{}]{
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
		Extract: func(details *domain.Details) (string, struct{}) {
			// Extract space details and handle space changes
			id := details.GetString(bundle.RelationKeyId)
			spaceId := details.GetString(bundle.RelationKeyTargetSpaceId)
			if spaceId != "" {
				// Check space status and initialize/uninitialize cache as needed
				spaceAccountStatus := details.GetInt64(bundle.RelationKeySpaceAccountStatus)
				spaceLocalStatus := details.GetInt64(bundle.RelationKeySpaceLocalStatus)
				isArchived := details.GetBool(bundle.RelationKeyIsArchived)
				isDeleted := details.GetBool(bundle.RelationKeyIsDeleted)

				// If space is active and not deleted/archived, initialize cache
				if !isDeleted && !isArchived && spaceAccountStatus == int64(model.SpaceStatus_Ok) && spaceLocalStatus == int64(model.SpaceStatus_Ok) {
					if err := s.initializeSpaceCache(spaceId); err != nil {
						log.Debugf("failed to initialize cache for space %s: %v", spaceId, err)
					}
				} else {
					// Otherwise, unsubscribe from the space
					s.unsubscribeFromSpace(spaceId)
				}
			}
			return id, struct{}{}
		},
		Update: func(key string, value domain.Value, data struct{}) struct{} {
			// Handle updates to space status fields
			switch key {
			case bundle.RelationKeyTargetSpaceId.String(),
				bundle.RelationKeySpaceAccountStatus.String(),
				bundle.RelationKeySpaceLocalStatus.String(),
				bundle.RelationKeyIsArchived.String(),
				bundle.RelationKeyIsDeleted.String():
				// We need to re-evaluate the space status
				// Since we don't have access to the full object here, we'll rely on the subscription remove event
				// or fetch the full object if needed
			}
			return data
		},
		Unset: func(keys []string, data struct{}) struct{} {
			// Nothing to unset for space monitoring
			return data
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
			switch key {
			case bundle.RelationKeyName.String():
				t.Name = value.String()
			case bundle.RelationKeyPluralName.String():
				t.PluralName = value.String()
			case bundle.RelationKeyIsArchived.String():
				t.Archived = value.Bool()
			case bundle.RelationKeyApiObjectKey.String():
				if apiKey := value.String(); apiKey != "" {
					t.Key = apiKey
				}
				// Add other field updates as needed
			}
			return t
		},
		Unset: func(keys []string, t *apimodel.Type) *apimodel.Type {
			for _, key := range keys {
				switch key {
				case bundle.RelationKeyName.String():
					t.Name = ""
				case bundle.RelationKeyPluralName.String():
					t.PluralName = ""
					// Add other field unsets as needed
				}
			}
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
			switch key {
			case bundle.RelationKeyName.String():
				prop.Name = value.String()
			case bundle.RelationKeyRelationFormat.String():
				prop.Format = RelationFormatToPropertyFormat[model.RelationFormat(value.Int64())]
			case bundle.RelationKeyApiObjectKey.String():
				if apiKey := value.String(); apiKey != "" {
					prop.Key = apiKey
				}
				// Add other field updates as needed
			}
			return prop
		},
		Unset: func(keys []string, prop *apimodel.Property) *apimodel.Property {
			for _, key := range keys {
				switch key {
				case bundle.RelationKeyName.String():
					prop.Name = ""
					// Add other field unsets as needed
				}
			}
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
			switch key {
			case bundle.RelationKeyName.String():
				tag.Name = value.String()
			case bundle.RelationKeyRelationOptionColor.String():
				tag.Color = apimodel.ColorOptionToColor[value.String()]
				// Add other field updates as needed
			}
			return tag
		},
		Unset: func(keys []string, tag *apimodel.Tag) *apimodel.Tag {
			for _, key := range keys {
				switch key {
				case bundle.RelationKeyName.String():
					tag.Name = ""
					// Add other field unsets as needed
				}
			}
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
	s.typeSubscriptions = make(map[string]*objectsubscription.ObjectSubscription[*apimodel.Type])

	for _, sub := range s.propertySubscriptions {
		sub.Close()
	}
	s.propertySubscriptions = make(map[string]*objectsubscription.ObjectSubscription[*apimodel.Property])

	for _, sub := range s.tagSubscriptions {
		sub.Close()
	}
	s.tagSubscriptions = make(map[string]*objectsubscription.ObjectSubscription[*apimodel.Tag])
}
