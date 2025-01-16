package objectcreator

import (
	"context"
	"fmt"

	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
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

	uniqueKey, err := getUniqueKeyOrGenerate(coresb.SmartBlockTypeObjectType, details)
	if err != nil {
		return "", nil, fmt.Errorf("getUniqueKeyOrGenerate: %w", err)
	}
	object := details.Copy()

	if !object.Has(bundle.RelationKeyRecommendedLayout) {
		object.SetInt64(bundle.RelationKeyRecommendedLayout, int64(model.ObjectType_basic))
	}
	if len(object.GetStringList(bundle.RelationKeyRecommendedRelations)) == 0 {
		err = s.fillRecommendedRelationsFromLayout(ctx, space, object)
		if err != nil {
			return "", nil, fmt.Errorf("fill recommended relations: %w", err)
		}
	}

	object.SetString(bundle.RelationKeyId, id)
	object.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_objectType))

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	id, newDetails, err = s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyObjectType}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	installingObjectTypeKey := domain.TypeKey(uniqueKey.InternalKey())
	err = s.installTemplatesForObjectType(space, installingObjectTypeKey)
	if err != nil {
		log.With("spaceID", space.Id(), "objectTypeKey", installingObjectTypeKey).Errorf("error while installing templates: %s", err)
	}
	return id, newDetails, nil
}

func (s *service) fillRecommendedRelationsFromLayout(ctx context.Context, space clientspace.Space, details *domain.Details) error {
	rawRecommendedLayout := details.GetInt64(bundle.RelationKeyRecommendedLayout)
	recommendedLayout, err := bundle.GetLayout(model.ObjectTypeLayout(int32(rawRecommendedLayout)))
	if err != nil {
		return fmt.Errorf("invalid recommended layout %d: %w", rawRecommendedLayout, err)
	}
	recommendedRelationKeys := make([]string, 0, len(recommendedLayout.RequiredRelations)+1)
	for _, rel := range recommendedLayout.RequiredRelations {
		recommendedRelationKeys = append(recommendedRelationKeys, rel.Key)
	}
	recommendedRelationIds, err := s.prepareRecommendedRelationIds(ctx, space, recommendedRelationKeys)
	if err != nil {
		return fmt.Errorf("prepare recommended relation ids: %w", err)
	}
	details.SetStringList(bundle.RelationKeyRecommendedRelations, recommendedRelationIds)
	return nil
}

func (s *service) prepareRecommendedRelationIds(ctx context.Context, space clientspace.Space, recommendedRelationKeys []string) ([]string, error) {
	descriptionRelationKey := bundle.RelationKeyDescription.String()
	if !slices.Contains(recommendedRelationKeys, descriptionRelationKey) {
		recommendedRelationKeys = append(recommendedRelationKeys, descriptionRelationKey)
	}
	recommendedRelationIDs := make([]string, 0, len(recommendedRelationKeys))
	relationsToInstall := make([]string, 0, len(recommendedRelationKeys))
	for _, relKey := range recommendedRelationKeys {
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, relKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create unique Key: %w", err)
		}
		relationsToInstall = append(relationsToInstall, domain.RelationKey(relKey).BundledURL())
		id, err := space.DeriveObjectID(ctx, uk)
		if err != nil {
			return nil, fmt.Errorf("failed to derive object id: %w", err)
		}
		recommendedRelationIDs = append(recommendedRelationIDs, id)
	}
	_, _, err := s.InstallBundledObjects(ctx, space, relationsToInstall, false)
	if err != nil {
		return nil, fmt.Errorf("install recommended relations: %w", err)
	}
	return recommendedRelationIDs, nil
}

func (s *service) installTemplatesForObjectType(spc clientspace.Space, typeKey domain.TypeKey) error {
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
