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

	propertyObjSub := objectsubscription.NewFromQueue(s.propertyQueue, s.createPropertySubscriptionParams())
	if err := propertyObjSub.Run(); err != nil {
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

	typeObjSub := objectsubscription.NewFromQueue(s.typeQueue, s.createTypeSubscriptionParams())
	if err := typeObjSub.Run(); err != nil {
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

	for _, record := range resp.Records {
		spaceId := record.GetString(bundle.RelationKeySpaceId)
		if spaceId == "" {
			continue
		}
		tag := s.getTagFromStruct(record.ToProto())
		s.cacheTag(spaceId, tag)
	}

	tagObjSub := objectsubscription.NewFromQueue(s.tagQueue, s.createTagSubscriptionParams())
	if err := tagObjSub.Run(); err != nil {
		return fmt.Errorf("failed to run tag object subscription: %w", err)
	}

	return nil
}

type propertyWithSpace struct {
	*apimodel.Property
	spaceId string
}

// refreshPropertyFromDetails re-fetches and updates property details
func (s *Service) refreshPropertyFromDetails(curEntry *propertyWithSpace) *propertyWithSpace {
	details, err := s.getObjectDetails(context.Background(), curEntry.spaceId, curEntry.Id)
	if err != nil {
		log.Errorf("failed to get property details for %s: %v", curEntry.Id, err)
		return curEntry
	}
	_, _, prop := s.getPropertyFromStruct(details)
	curEntry.Property = prop
	return curEntry
}

// refreshTypeFromDetails re-fetches and updates type details
func (s *Service) refreshTypeFromDetails(curEntry *typeWithSpace) *typeWithSpace {
	details, err := s.getObjectDetails(context.Background(), curEntry.spaceId, curEntry.Id)
	if err != nil {
		log.Errorf("failed to get type details for %s: %v", curEntry.Id, err)
		return curEntry
	}
	propertyMap := s.getPropertyMap(curEntry.spaceId)
	_, _, t := s.getTypeFromStruct(details, propertyMap)
	curEntry.Type = t
	return curEntry
}

// refreshTagFromDetails re-fetches and updates tag details
func (s *Service) refreshTagFromDetails(curEntry *tagWithSpace) *tagWithSpace {
	details, err := s.getObjectDetails(context.Background(), curEntry.spaceId, curEntry.Id)
	if err != nil {
		log.Errorf("failed to get tag details for %s: %v", curEntry.Id, err)
		return curEntry
	}
	tag := s.getTagFromStruct(details)
	curEntry.Tag = tag
	return curEntry
}

// createPropertySubscriptionParams creates the subscription parameters for properties
func (s *Service) createPropertySubscriptionParams() objectsubscription.SubscriptionParams[*propertyWithSpace] {
	return objectsubscription.SubscriptionParams[*propertyWithSpace]{
		SetDetails: func(details *domain.Details) (id string, entry *propertyWithSpace) {
			spaceId := details.GetString(bundle.RelationKeySpaceId)
			_, _, prop := s.getPropertyFromStruct(details.ToProto())
			return prop.Id, &propertyWithSpace{
				Property: prop,
				spaceId:  spaceId,
			}
		},
		UpdateKey: func(relationKey string, relationValue domain.Value, curEntry *propertyWithSpace) *propertyWithSpace {
			return s.refreshPropertyFromDetails(curEntry)
		},
		RemoveKeys: func(keys []string, curEntry *propertyWithSpace) *propertyWithSpace {
			return s.refreshPropertyFromDetails(curEntry)
		},
		OnAdded: func(id string, entry *propertyWithSpace) {
			s.subscriptionsMu.Lock()
			s.cacheProperty(entry.spaceId, entry.Property)
			s.subscriptionsMu.Unlock()
		},
		OnRemoved: func(id string, entry *propertyWithSpace) {
			s.subscriptionsMu.Lock()
			if spaceCache, exists := s.propertyCache[entry.spaceId]; exists {
				delete(spaceCache, entry.Id)
				delete(spaceCache, entry.RelationKey)
				delete(spaceCache, entry.Key)
			}
			s.subscriptionsMu.Unlock()
		},
	}
}

// typeWithSpace wraps a type with its space ID for cross-space tracking
type typeWithSpace struct {
	*apimodel.Type
	spaceId string
}

// createTypeSubscriptionParams creates the subscription parameters for types
func (s *Service) createTypeSubscriptionParams() objectsubscription.SubscriptionParams[*typeWithSpace] {
	return objectsubscription.SubscriptionParams[*typeWithSpace]{
		SetDetails: func(details *domain.Details) (id string, entry *typeWithSpace) {
			spaceId := details.GetString(bundle.RelationKeySpaceId)
			propertyMap := s.getPropertyMap(spaceId)
			_, _, t := s.getTypeFromStruct(details.ToProto(), propertyMap)
			return t.Id, &typeWithSpace{
				Type:    t,
				spaceId: spaceId,
			}
		},
		UpdateKey: func(relationKey string, relationValue domain.Value, curEntry *typeWithSpace) *typeWithSpace {
			return s.refreshTypeFromDetails(curEntry)
		},
		RemoveKeys: func(keys []string, curEntry *typeWithSpace) *typeWithSpace {
			return s.refreshTypeFromDetails(curEntry)
		},
		OnAdded: func(id string, entry *typeWithSpace) {
			s.subscriptionsMu.Lock()
			s.cacheType(entry.spaceId, entry.Type)
			s.subscriptionsMu.Unlock()
		},
		OnRemoved: func(id string, entry *typeWithSpace) {
			s.subscriptionsMu.Lock()
			if spaceCache, exists := s.typeCache[entry.spaceId]; exists {
				// Remove all keys pointing to this type
				delete(spaceCache, entry.Id)
				delete(spaceCache, entry.UniqueKey)
				delete(spaceCache, entry.Key)
			}
			s.subscriptionsMu.Unlock()
		},
	}
}

// tagWithSpace wraps a tag with its space ID for cross-space tracking
type tagWithSpace struct {
	*apimodel.Tag
	spaceId string
}

// createTagSubscriptionParams creates the subscription parameters for tags
func (s *Service) createTagSubscriptionParams() objectsubscription.SubscriptionParams[*tagWithSpace] {
	return objectsubscription.SubscriptionParams[*tagWithSpace]{
		SetDetails: func(details *domain.Details) (id string, entry *tagWithSpace) {
			spaceId := details.GetString(bundle.RelationKeySpaceId)
			tag := s.getTagFromStruct(details.ToProto())
			return tag.Id, &tagWithSpace{
				Tag:     tag,
				spaceId: spaceId,
			}
		},
		UpdateKey: func(relationKey string, relationValue domain.Value, curEntry *tagWithSpace) *tagWithSpace {
			return s.refreshTagFromDetails(curEntry)
		},
		RemoveKeys: func(keys []string, curEntry *tagWithSpace) *tagWithSpace {
			return s.refreshTagFromDetails(curEntry)
		},
		OnAdded: func(id string, entry *tagWithSpace) {
			s.subscriptionsMu.Lock()
			s.cacheTag(entry.spaceId, entry.Tag)
			s.subscriptionsMu.Unlock()
		},
		OnRemoved: func(id string, entry *tagWithSpace) {
			s.subscriptionsMu.Lock()
			if spaceCache, exists := s.tagCache[entry.spaceId]; exists {
				delete(spaceCache, entry.Id)
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
