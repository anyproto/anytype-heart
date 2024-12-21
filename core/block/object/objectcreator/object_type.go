package objectcreator

import (
	"context"
	"errors"
	"fmt"

	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

var (
	defaultRecommendedFeaturedRelationKeys = []domain.RelationKey{
		bundle.RelationKeyType,
		bundle.RelationKeyTag,
		bundle.RelationKeyBacklinks,
	}

	defaultRecommendedRelationKeys = []domain.RelationKey{
		bundle.RelationKeyCreatedDate,
		bundle.RelationKeyCreator,
		bundle.RelationKeyLastModifiedDate,
		bundle.RelationKeyLastModifiedBy,
		bundle.RelationKeyLastOpenedDate,
		bundle.RelationKeyLinks,
	}

	// relationsToExclude = []domain.RelationKey{
	// 	bundle.RelationKeyDescription,
	// }

	errRecommendedRelationsAlreadyFilled = fmt.Errorf("recommended featured relations are already filled")
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

	keys, isAlreadyFilled, err := fillRecommendedRelations(ctx, space, object)
	if err != nil {
		return "", nil, fmt.Errorf("fill recommended relations: %w", err)
	}
	if !isAlreadyFilled {
		err = s.installRecommendedRelations(ctx, space, keys)
		if err != nil {
			return "", nil, fmt.Errorf("install recommended relations: %w", err)
		}
	}

	object.SetString(bundle.RelationKeyId, id)
	object.SetInt64(bundle.RelationKeyResolvedLayout, int64(model.ObjectType_objectType))

	createState := state.NewDocWithUniqueKey("", nil, uniqueKey).(*state.State)
	createState.SetDetails(object)
	id, newDetails, err = s.CreateSmartBlockFromStateInSpace(ctx, space, []domain.TypeKey{bundle.TypeKeyObjectType}, createState)
	if err != nil {
		return "", nil, fmt.Errorf("create smartblock from state: %w", err)
	}

	installingObjectTypeKey := domain.TypeKey(uniqueKey.InternalKey())
	err = s.createTemplatesForObjectType(space, installingObjectTypeKey)
	if err != nil {
		log.With("spaceID", space.Id(), "objectTypeKey", installingObjectTypeKey).Errorf("error while installing templates: %s", err)
	}
	return id, newDetails, nil
}

// fillRecommendedRelations fills recommendedRelations and recommendedFeaturedRelations based on object's details
// If these relations are already filled with correct ids, isAlreadyFilled = true is returned
func fillRecommendedRelations(ctx context.Context, spc clientspace.Space, details *domain.Details) (keys []domain.RelationKey, isAlreadyFilled bool, err error) {
	keys, err = getRelationKeysFromDetails(details)
	if err != nil {
		if errors.Is(err, errRecommendedRelationsAlreadyFilled) {
			return nil, true, nil
		}
		return nil, false, fmt.Errorf("get recommended relation keys: %w", err)
	}

	// we should include default system recommended relations and exclude default recommended featured relations
	keys = lo.Uniq(append(keys, defaultRecommendedRelationKeys...))
	keys = slices.DeleteFunc(keys, func(key domain.RelationKey) bool {
		return slices.Contains(defaultRecommendedFeaturedRelationKeys, key)
	})

	relationIds, err := prepareRelationIds(ctx, spc, keys)
	if err != nil {
		return nil, false, fmt.Errorf("prepare recommended relation ids: %w", err)
	}
	details.SetStringList(bundle.RelationKeyRecommendedRelations, relationIds)

	featuredRelationIds, err := prepareRelationIds(ctx, spc, defaultRecommendedFeaturedRelationKeys)
	if err != nil {
		return nil, false, fmt.Errorf("prepare recommended featured relation ids: %w", err)
	}
	details.SetStringList(bundle.RelationKeyRecommendedFeaturedRelations, featuredRelationIds)

	return append(keys, defaultRecommendedFeaturedRelationKeys...), false, nil
}

func getRelationKeysFromDetails(details *domain.Details) ([]domain.RelationKey, error) {
	bundledRelationIds := details.GetStringList(bundle.RelationKeyRecommendedRelations)
	if len(bundledRelationIds) == 0 {
		rawRecommendedLayout := details.GetInt64(bundle.RelationKeyRecommendedLayout)
		// nolint: gosec
		recommendedLayout, err := bundle.GetLayout(model.ObjectTypeLayout(rawRecommendedLayout))
		if err != nil {
			return nil, fmt.Errorf("invalid recommended layout %d: %w", rawRecommendedLayout, err)
		}
		keys := make([]domain.RelationKey, 0, len(recommendedLayout.RequiredRelations))
		for _, rel := range recommendedLayout.RequiredRelations {
			keys = append(keys, domain.RelationKey(rel.Key))
		}
		return keys, nil
	}

	keys := make([]domain.RelationKey, 0, len(bundledRelationIds))
	for i, id := range bundledRelationIds {
		key, err := bundle.RelationKeyFromID(id)
		if err == nil {
			// TODO: use Contains when we have more relations to exclude
			// if !slices.Contains(relationsToExclude, key) {
			if key != bundle.RelationKeyDescription {
				keys = append(keys, key)
			}
			continue
		}
		if i == 0 {
			// if we fail to parse 1st bundled relation id, details are already filled with correct ids
			return nil, errRecommendedRelationsAlreadyFilled
		}
		return nil, fmt.Errorf("relation key from id: %w", err)
	}
	return keys, nil
}

func prepareRelationIds(ctx context.Context, space clientspace.Space, relationKeys []domain.RelationKey) ([]string, error) {
	relationIds := make([]string, 0, len(relationKeys))
	for _, key := range relationKeys {
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, key.String())
		if err != nil {
			return nil, fmt.Errorf("failed to create unique Key: %w", err)
		}
		id, err := space.DeriveObjectID(ctx, uk)
		if err != nil {
			return nil, fmt.Errorf("failed to derive object id: %w", err)
		}
		relationIds = append(relationIds, id)
	}
	return relationIds, nil
}

func (s *service) installRecommendedRelations(ctx context.Context, space clientspace.Space, relationKeys []domain.RelationKey) error {
	bundledRelationIds := make([]string, len(relationKeys))
	for i, key := range relationKeys {
		bundledRelationIds[i] = key.BundledURL()
	}
	_, _, err := s.InstallBundledObjects(ctx, space, bundledRelationIds, false)
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
