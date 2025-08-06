package service

import (
	"fmt"

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

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
	if s.subscriptions.properties.queue != nil {
		return nil // Already subscribed
	}

	s.subscriptions.properties.queue = mb.New[*pb.EventMessage](0)

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
		InternalQueue:     s.subscriptions.properties.queue,
	}, crossspacesub.NoOpPredicate())

	if err != nil {
		return fmt.Errorf("failed to subscribe to cross-space properties: %w", err)
	}

	s.subscriptions.properties.subId = resp.SubId

	for _, record := range resp.Records {
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if spaceId == "" {
			continue
		}
		_, _, prop := s.getPropertyFromStruct(record.ToProto())
		s.cache.cacheProperty(spaceId, prop)
	}

	s.subscriptions.properties.objSub = objectsubscription.NewFromQueue(s.subscriptions.properties.queue, s.createPropertySubscriptionParams(), resp.Records)
	if err := s.subscriptions.properties.objSub.(*objectsubscription.ObjectSubscription[*propertyWithSpace]).Run(); err != nil {
		return fmt.Errorf("failed to run property object subscription: %w", err)
	}

	return nil
}

// subscribeToCrossSpaceTypes subscribes to type changes across all active spaces
func (s *Service) subscribeToCrossSpaceTypes() error {
	if s.subscriptions.types.queue != nil {
		return nil // Already subscribed
	}

	s.subscriptions.types.queue = mb.New[*pb.EventMessage](0)

	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_objectType)),
		},
		{
			RelationKey: bundle.RelationKeyIsHidden,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
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
		InternalQueue:     s.subscriptions.types.queue,
	}, crossspacesub.NoOpPredicate())

	if err != nil {
		return fmt.Errorf("failed to subscribe to cross-space types: %w", err)
	}

	s.subscriptions.types.subId = resp.SubId

	for _, record := range resp.Records {
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if spaceId == "" {
			continue
		}
		propertyMap := s.cache.getProperties(spaceId)
		_, _, t := s.getTypeFromStruct(record.ToProto(), propertyMap)
		s.cache.cacheType(spaceId, t)
	}

	s.subscriptions.types.objSub = objectsubscription.NewFromQueue(s.subscriptions.types.queue, s.createTypeSubscriptionParams(), resp.Records)
	if err := s.subscriptions.types.objSub.(*objectsubscription.ObjectSubscription[*typeWithSpace]).Run(); err != nil {
		return fmt.Errorf("failed to run type object subscription: %w", err)
	}

	return nil
}

// subscribeToCrossSpaceTags subscribes to tag changes across all active spaces
func (s *Service) subscribeToCrossSpaceTags() error {
	if s.subscriptions.tags.queue != nil {
		return nil
	}

	s.subscriptions.tags.queue = mb.New[*pb.EventMessage](0)

	filters := []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List(util.LayoutsToIntArgs(util.TagLayouts)),
		},
		{
			RelationKey: bundle.RelationKeyIsHidden,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		},
	}

	resp, err := s.crossSpaceSubService.Subscribe(subscription.SubscribeRequest{
		SubId:   "api.tags.crossspace",
		Filters: filters,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyApiObjectKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationOptionColor.String(),
			bundle.RelationKeySpaceId.String(),
		},
		NoDepSubscription: true,
		InternalQueue:     s.subscriptions.tags.queue,
	}, crossspacesub.NoOpPredicate())

	if err != nil {
		return fmt.Errorf("failed to subscribe to cross-space tags: %w", err)
	}

	s.subscriptions.tags.subId = resp.SubId

	for _, record := range resp.Records {
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if spaceId == "" {
			continue
		}
		tag := s.getTagFromStruct(record.ToProto())
		s.cache.cacheTag(spaceId, tag)
	}

	s.subscriptions.tags.objSub = objectsubscription.NewFromQueue(s.subscriptions.tags.queue, s.createTagSubscriptionParams(), resp.Records)
	if err := s.subscriptions.tags.objSub.(*objectsubscription.ObjectSubscription[*tagWithSpace]).Run(); err != nil {
		return fmt.Errorf("failed to run tag object subscription: %w", err)
	}

	return nil
}

type propertyWithSpace struct {
	details *domain.Details
	spaceId string
}

// createPropertySubscriptionParams creates the subscription parameters for properties
func (s *Service) createPropertySubscriptionParams() objectsubscription.SubscriptionParams[*propertyWithSpace] {
	return objectsubscription.SubscriptionParams[*propertyWithSpace]{
		SetDetails: func(details *domain.Details) (id string, entry *propertyWithSpace) {
			spaceId := details.GetString(bundle.RelationKeySpaceId)
			_, _, prop := s.getPropertyFromStruct(details.ToProto())
			s.cache.cacheProperty(spaceId, prop)
			return details.GetString(bundle.RelationKeyId), &propertyWithSpace{
				details: details,
				spaceId: spaceId,
			}
		},
		UpdateKey: func(relationKey string, relationValue domain.Value, curEntry *propertyWithSpace) *propertyWithSpace {
			curEntry.details.Set(domain.RelationKey(relationKey), relationValue)
			_, _, prop := s.getPropertyFromStruct(curEntry.details.ToProto())
			s.cache.cacheProperty(curEntry.spaceId, prop)
			return curEntry
		},
		RemoveKeys: func(keys []string, curEntry *propertyWithSpace) *propertyWithSpace {
			for _, key := range keys {
				curEntry.details.Delete(domain.RelationKey(key))
			}
			_, _, prop := s.getPropertyFromStruct(curEntry.details.ToProto())
			s.cache.cacheProperty(curEntry.spaceId, prop)
			return curEntry
		},
		OnRemoved: func(id string, entry *propertyWithSpace) {
			_, _, prop := s.getPropertyFromStruct(entry.details.ToProto())
			s.cache.removeProperty(entry.spaceId, prop.Id, prop.RelationKey, prop.Key)
		},
	}
}

type typeWithSpace struct {
	details *domain.Details
	spaceId string
}

// createTypeSubscriptionParams creates the subscription parameters for types
func (s *Service) createTypeSubscriptionParams() objectsubscription.SubscriptionParams[*typeWithSpace] {
	return objectsubscription.SubscriptionParams[*typeWithSpace]{
		SetDetails: func(details *domain.Details) (id string, entry *typeWithSpace) {
			spaceId := details.GetString(bundle.RelationKeySpaceId)
			propertyMap := s.cache.getProperties(spaceId)
			_, _, t := s.getTypeFromStruct(details.ToProto(), propertyMap)
			s.cache.cacheType(spaceId, t)
			return details.GetString(bundle.RelationKeyId), &typeWithSpace{
				details: details,
				spaceId: spaceId,
			}
		},
		UpdateKey: func(relationKey string, relationValue domain.Value, curEntry *typeWithSpace) *typeWithSpace {
			curEntry.details.Set(domain.RelationKey(relationKey), relationValue)
			propertyMap := s.cache.getProperties(curEntry.spaceId)
			_, _, t := s.getTypeFromStruct(curEntry.details.ToProto(), propertyMap)
			s.cache.cacheType(curEntry.spaceId, t)
			return curEntry
		},
		RemoveKeys: func(keys []string, curEntry *typeWithSpace) *typeWithSpace {
			for _, key := range keys {
				curEntry.details.Delete(domain.RelationKey(key))
			}
			propertyMap := s.cache.getProperties(curEntry.spaceId)
			_, _, t := s.getTypeFromStruct(curEntry.details.ToProto(), propertyMap)
			s.cache.cacheType(curEntry.spaceId, t)
			return curEntry
		},
		OnRemoved: func(id string, entry *typeWithSpace) {
			propertyMap := s.cache.getProperties(entry.spaceId)
			_, _, t := s.getTypeFromStruct(entry.details.ToProto(), propertyMap)
			s.cache.removeType(entry.spaceId, t.Id, t.UniqueKey, t.Key)
		},
	}
}

type tagWithSpace struct {
	details *domain.Details
	spaceId string
}

// createTagSubscriptionParams creates the subscription parameters for tags
func (s *Service) createTagSubscriptionParams() objectsubscription.SubscriptionParams[*tagWithSpace] {
	return objectsubscription.SubscriptionParams[*tagWithSpace]{
		SetDetails: func(details *domain.Details) (id string, entry *tagWithSpace) {
			spaceId := details.GetString(bundle.RelationKeySpaceId)
			tag := s.getTagFromStruct(details.ToProto())
			s.cache.cacheTag(spaceId, tag)
			return details.GetString(bundle.RelationKeyId), &tagWithSpace{
				details: details,
				spaceId: spaceId,
			}
		},
		UpdateKey: func(relationKey string, relationValue domain.Value, curEntry *tagWithSpace) *tagWithSpace {
			curEntry.details.Set(domain.RelationKey(relationKey), relationValue)
			tag := s.getTagFromStruct(curEntry.details.ToProto())
			s.cache.cacheTag(curEntry.spaceId, tag)
			return curEntry
		},
		RemoveKeys: func(keys []string, curEntry *tagWithSpace) *tagWithSpace {
			for _, key := range keys {
				curEntry.details.Delete(domain.RelationKey(key))
			}
			tag := s.getTagFromStruct(curEntry.details.ToProto())
			s.cache.cacheTag(curEntry.spaceId, tag)
			return curEntry
		},
		OnRemoved: func(id string, entry *tagWithSpace) {
			tag := s.getTagFromStruct(entry.details.ToProto())
			s.cache.removeTag(entry.spaceId, tag.Id, tag.UniqueKey, tag.Key)
		},
	}
}

// Stop unsubscribes from all cross-space subscriptions and cleans up
func (s *Service) Stop() {
	if s.componentCtxCancel != nil {
		s.componentCtxCancel()
	}

	// Close all subscriptions
	s.subscriptions.close()

	// Unsubscribe from cross-space subscriptions
	if s.subscriptions.properties.subId != "" {
		if err := s.crossSpaceSubService.Unsubscribe(s.subscriptions.properties.subId); err != nil {
			log.Errorf("Failed to unsubscribe from cross-space properties: %v", err)
		}
	}

	if s.subscriptions.types.subId != "" {
		if err := s.crossSpaceSubService.Unsubscribe(s.subscriptions.types.subId); err != nil {
			log.Errorf("Failed to unsubscribe from cross-space types: %v", err)
		}
	}

	if s.subscriptions.tags.subId != "" {
		if err := s.crossSpaceSubService.Unsubscribe(s.subscriptions.tags.subId); err != nil {
			log.Errorf("Failed to unsubscribe from cross-space tags: %v", err)
		}
	}

	// Clear cache
	s.cache.clear()
}
