package objectcreator

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/order"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) createObjectType(ctx context.Context, space clientspace.Space, details *domain.Details) (id string, newDetails *domain.Details, err error) {
	if details == nil {
		return "", nil, fmt.Errorf("create object type: no data")
	}

	uniqueKey, wasGenerated, err := getUniqueKeyOrGenerate(coresb.SmartBlockTypeObjectType, details)
	if err != nil {
		return "", nil, fmt.Errorf("getUniqueKeyOrGenerate: %w", err)
	}
	object := details.Copy()

	var objectKey string
	if !wasGenerated {
		objectKey = uniqueKey.InternalKey()
	}
	injectApiObjectKey(object, objectKey)

	if !object.Has(bundle.RelationKeyRecommendedLayout) {
		object.SetInt64(bundle.RelationKeyRecommendedLayout, int64(model.ObjectType_basic))
	}

	keys, isAlreadyFilled, err := relationutils.FillRecommendedRelations(ctx, space, object, domain.TypeKey(uniqueKey.InternalKey()))
	if err != nil {
		return "", nil, fmt.Errorf("fill recommended relations: %w", err)
	}
	if !isAlreadyFilled {
		err = s.installRecommendedRelations(ctx, space, keys)
		if err != nil {
			return "", nil, fmt.Errorf("install recommended relations: %w", err)
		}
	}
	if !object.Has(bundle.RelationKeyCreatedDate) {
		object.SetInt64(bundle.RelationKeyCreatedDate, time.Now().Unix())
	}

	object.SetString(bundle.RelationKeyId, id)
	object.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_objectType))

	if err = s.setOrderId(object, space); err != nil {
		log.With("spaceID", space.Id()).Errorf("failed to set orderId: %v", err)
	}

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	setOriginalCreatedTimestamp(createState, details)
	id, newDetails, err = s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyObjectType}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	typeKey := domain.TypeKey(uniqueKey.InternalKey())

	if err = s.createTemplatesForObjectType(space, typeKey); err != nil {
		log.With("spaceID", space.Id(), "objectTypeKey", typeKey).Errorf("error while installing templates: %s", err)
	}
	return id, newDetails, nil
}

func (s *service) installRecommendedRelations(ctx context.Context, space clientspace.Space, relationKeys []domain.RelationKey) error {
	bundledRelationIds := make([]string, len(relationKeys))
	for i, key := range relationKeys {
		bundledRelationIds[i] = key.BundledURL()
	}
	_, _, err := s.InstallBundledObjects(ctx, space, bundledRelationIds)
	return err
}

func (s *service) createTemplatesForObjectType(spc clientspace.Space, typeKey domain.TypeKey) error {
	bundledTemplates, err := s.objectStore.SpaceIndex(spc.Id()).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyType,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(bundle.TypeKeyTemplate.BundledURL()),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(typeKey.BundledURL()),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query bundled templates: %w", err)
	}

	installedTemplatesIDs, err := s.listInstalledTemplatesForType(spc, typeKey)
	if err != nil {
		return fmt.Errorf("list installed templates: %w", err)
	}

	for _, record := range bundledTemplates {
		id := record.Details.GetString(bundle.RelationKeyId)
		if _, exists := installedTemplatesIDs[id]; exists {
			continue
		}

		_, err := s.templateService.TemplateCloneInSpace(spc, id)
		if err != nil {
			return fmt.Errorf("clone template: %w", err)
		}
	}
	return nil
}

func (s *service) listInstalledTemplatesForType(spc clientspace.Space, typeKey domain.TypeKey) (map[string]struct{}, error) {
	templateTypeID, err := spc.GetTypeIdByKey(context.Background(), bundle.TypeKeyTemplate)
	if err != nil {
		return nil, fmt.Errorf("get template type id by key: %w", err)
	}
	targetObjectTypeID, err := spc.GetTypeIdByKey(context.Background(), typeKey)
	if err != nil {
		return nil, fmt.Errorf("get type id by key: %w", err)
	}
	alreadyInstalledTemplates, err := s.objectStore.SpaceIndex(spc.Id()).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyType,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(templateTypeID),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(targetObjectTypeID),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	existingTemplatesMap := map[string]struct{}{}
	for _, rec := range alreadyInstalledTemplates {
		sourceObject := rec.Details.GetString(bundle.RelationKeySourceObject)
		if sourceObject != "" {
			existingTemplatesMap[sourceObject] = struct{}{}
		}
	}
	return existingTemplatesMap, nil
}

func (s *service) setOrderId(details *domain.Details, spc clientspace.Space) error {
	records, err := s.objectStore.SpaceIndex(spc.Id()).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(model.ObjectType_objectType),
			},
			{
				RelationKey: bundle.RelationKeyOrderId,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
		},
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyOrderId,
			Type:        model.BlockContentDataviewSort_Asc,
			NoCollate:   true,
		}},
		Limit: 1,
	})

	if err != nil {
		return fmt.Errorf("failed to query object types with orders: %w", err)
	}

	if len(records) == 0 {
		return nil
	}

	smallestOrderId := records[0].Details.GetString(bundle.RelationKeyOrderId)
	if smallestOrderId == "" {
		return nil
	}

	details.SetString(bundle.RelationKeyOrderId, order.GetSmallestOrder(smallestOrderId))
	return nil
}
