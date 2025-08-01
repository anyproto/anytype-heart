package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
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
		InternalQueue:     s.propertyQueue,
	}, crossspacesub.NoOpPredicate())

	if err != nil {
		return fmt.Errorf("failed to subscribe to cross-space properties: %w", err)
	}

	s.propertySubId = resp.SubId

	for _, record := range resp.Records {
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if spaceId == "" {
			continue
		}
		_, _, prop := s.getPropertyFromStruct(record.ToProto())
		s.cacheProperty(spaceId, prop)
	}

	s.propertyObjSub = objectsubscription.NewFromQueue(s.propertyQueue, s.createPropertySubscriptionParams(), resp.Records)
	if err := s.propertyObjSub.(*objectsubscription.ObjectSubscription[*propertyWithSpace]).Run(); err != nil {
		return fmt.Errorf("failed to run property object subscription: %w", err)
	}

	return nil
}

// subscribeToCrossSpaceTypes subscribes to type changes across all active spaces
func (s *Service) subscribeToCrossSpaceTypes() error {
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
		InternalQueue:     s.typeQueue,
	}, crossspacesub.NoOpPredicate())

	if err != nil {
		return fmt.Errorf("failed to subscribe to cross-space types: %w", err)
	}

	s.typeSubId = resp.SubId

	for _, record := range resp.Records {
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if spaceId == "" {
			continue
		}
		propertyMap := s.getPropertyMap(spaceId)
		_, _, t := s.getTypeFromStruct(record.ToProto(), propertyMap)
		s.cacheType(spaceId, t)
	}

	s.typeObjSub = objectsubscription.NewFromQueue(s.typeQueue, s.createTypeSubscriptionParams(), resp.Records)
	if err := s.typeObjSub.(*objectsubscription.ObjectSubscription[*typeWithSpace]).Run(); err != nil {
		return fmt.Errorf("failed to run type object subscription: %w", err)
	}

	return nil
}

// subscribeToCrossSpaceTags subscribes to tag changes across all active spaces
func (s *Service) subscribeToCrossSpaceTags() error {
	if s.tagQueue != nil {
		return nil
	}

	s.tagQueue = mb.New[*pb.EventMessage](0)
	s.tagCache = make(map[string]map[string]*apimodel.Tag) // spaceId -> id -> Tag

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

	for _, record := range resp.Records {
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if spaceId == "" {
			continue
		}
		tag := s.getTagFromStruct(record.ToProto())
		s.cacheTag(spaceId, tag)
	}

	s.tagObjSub = objectsubscription.NewFromQueue(s.tagQueue, s.createTagSubscriptionParams(), resp.Records)
	if err := s.tagObjSub.(*objectsubscription.ObjectSubscription[*tagWithSpace]).Run(); err != nil {
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
			s.subscriptionsMu.Lock()
			s.cacheProperty(spaceId, prop)
			s.subscriptionsMu.Unlock()
			return details.GetString(bundle.RelationKeyId), &propertyWithSpace{
				details: details,
				spaceId: spaceId,
			}
		},
		UpdateKey: func(relationKey string, relationValue domain.Value, curEntry *propertyWithSpace) *propertyWithSpace {
			curEntry.details.Set(domain.RelationKey(relationKey), relationValue)
			_, _, prop := s.getPropertyFromStruct(curEntry.details.ToProto())
			s.subscriptionsMu.Lock()
			s.cacheProperty(curEntry.spaceId, prop)
			s.subscriptionsMu.Unlock()
			return curEntry
		},
		RemoveKeys: func(keys []string, curEntry *propertyWithSpace) *propertyWithSpace {
			for _, key := range keys {
				curEntry.details.Delete(domain.RelationKey(key))
			}
			_, _, prop := s.getPropertyFromStruct(curEntry.details.ToProto())
			s.subscriptionsMu.Lock()
			s.cacheProperty(curEntry.spaceId, prop)
			s.subscriptionsMu.Unlock()
			return curEntry
		},
		OnRemoved: func(id string, entry *propertyWithSpace) {
			_, _, prop := s.getPropertyFromStruct(entry.details.ToProto())
			s.subscriptionsMu.Lock()
			if spaceCache, exists := s.propertyCache[entry.spaceId]; exists {
				delete(spaceCache, prop.Id)
				delete(spaceCache, prop.RelationKey)
				delete(spaceCache, prop.Key)
			}
			s.subscriptionsMu.Unlock()
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
			propertyMap := s.getPropertyMap(spaceId)
			_, _, t := s.getTypeFromStruct(details.ToProto(), propertyMap)
			s.subscriptionsMu.Lock()
			s.cacheType(spaceId, t)
			s.subscriptionsMu.Unlock()
			return details.GetString(bundle.RelationKeyId), &typeWithSpace{
				details: details,
				spaceId: spaceId,
			}
		},
		UpdateKey: func(relationKey string, relationValue domain.Value, curEntry *typeWithSpace) *typeWithSpace {
			curEntry.details.Set(domain.RelationKey(relationKey), relationValue)
			propertyMap := s.getPropertyMap(curEntry.spaceId)
			_, _, t := s.getTypeFromStruct(curEntry.details.ToProto(), propertyMap)
			s.subscriptionsMu.Lock()
			s.cacheType(curEntry.spaceId, t)
			s.subscriptionsMu.Unlock()
			return curEntry
		},
		RemoveKeys: func(keys []string, curEntry *typeWithSpace) *typeWithSpace {
			for _, key := range keys {
				curEntry.details.Delete(domain.RelationKey(key))
			}
			propertyMap := s.getPropertyMap(curEntry.spaceId)
			_, _, t := s.getTypeFromStruct(curEntry.details.ToProto(), propertyMap)
			s.subscriptionsMu.Lock()
			s.cacheType(curEntry.spaceId, t)
			s.subscriptionsMu.Unlock()
			return curEntry
		},
		OnRemoved: func(id string, entry *typeWithSpace) {
			propertyMap := s.getPropertyMap(entry.spaceId)
			_, _, t := s.getTypeFromStruct(entry.details.ToProto(), propertyMap)
			s.subscriptionsMu.Lock()
			if spaceCache, exists := s.typeCache[entry.spaceId]; exists {
				// Remove all keys pointing to this type
				delete(spaceCache, t.Id)
				delete(spaceCache, t.UniqueKey)
				delete(spaceCache, t.Key)
			}
			s.subscriptionsMu.Unlock()
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
			s.subscriptionsMu.Lock()
			s.cacheTag(spaceId, tag)
			s.subscriptionsMu.Unlock()
			return details.GetString(bundle.RelationKeyId), &tagWithSpace{
				details: details,
				spaceId: spaceId,
			}
		},
		UpdateKey: func(relationKey string, relationValue domain.Value, curEntry *tagWithSpace) *tagWithSpace {
			curEntry.details.Set(domain.RelationKey(relationKey), relationValue)
			tag := s.getTagFromStruct(curEntry.details.ToProto())
			s.subscriptionsMu.Lock()
			s.cacheTag(curEntry.spaceId, tag)
			s.subscriptionsMu.Unlock()
			return curEntry
		},
		RemoveKeys: func(keys []string, curEntry *tagWithSpace) *tagWithSpace {
			for _, key := range keys {
				curEntry.details.Delete(domain.RelationKey(key))
			}
			tag := s.getTagFromStruct(curEntry.details.ToProto())
			s.subscriptionsMu.Lock()
			s.cacheTag(curEntry.spaceId, tag)
			s.subscriptionsMu.Unlock()
			return curEntry
		},
		OnRemoved: func(id string, entry *tagWithSpace) {
			s.subscriptionsMu.Lock()
			if spaceCache, exists := s.tagCache[entry.spaceId]; exists {
				delete(spaceCache, id)
			}
			s.subscriptionsMu.Unlock()
		},
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

// Stop unsubscribes from all cross-space subscriptions and cleans up
func (s *Service) Stop() {
	if s.componentCtxCancel != nil {
		s.componentCtxCancel()
	}

	// Close object subscriptions first
	if s.propertyObjSub != nil {
		s.propertyObjSub.Close()
	}
	if s.typeObjSub != nil {
		s.typeObjSub.Close()
	}
	if s.tagObjSub != nil {
		s.tagObjSub.Close()
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

	if s.propertyQueue != nil {
		s.propertyQueue.Close()
	}
	if s.typeQueue != nil {
		s.typeQueue.Close()
	}
	if s.tagQueue != nil {
		s.tagQueue.Close()
	}

	s.propertyCache = nil
	s.typeCache = nil
	s.tagCache = nil
}
