package objectcreator

import (
	"context"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) createObjectType(ctx context.Context, space space.Space, details *types.Struct) (id string, newDetails *types.Struct, err error) {
	if details == nil || details.Fields == nil {
		return "", nil, fmt.Errorf("create object type: no data")
	}

	uniqueKey, err := getUniqueKeyOrGenerate(coresb.SmartBlockTypeObjectType, details)
	if err != nil {
		return "", nil, fmt.Errorf("getUniqueKeyOrGenerate: %w", err)
	}
	details.Fields[bundle.RelationKeyUniqueKey.String()] = pbtypes.String(uniqueKey.Marshal())

	object := pbtypes.CopyStruct(details)
	rawRecommendedLayout := pbtypes.GetInt64(details, bundle.RelationKeyRecommendedLayout.String())
	recommendedLayout, err := bundle.GetLayout(model.ObjectTypeLayout(int32(rawRecommendedLayout)))
	if err != nil {
		return "", nil, fmt.Errorf("invalid recommended layout %d: %w", rawRecommendedLayout, err)
	}

	recommendedRelationKeys := make([]string, 0, len(recommendedLayout.RequiredRelations))
	for _, rel := range recommendedLayout.RequiredRelations {
		recommendedRelationKeys = append(recommendedRelationKeys, rel.Key)
	}
	recommendedRelationIDs := make([]string, 0, len(recommendedRelationKeys))
	for _, relKey := range recommendedRelationKeys {
		// TODO Install relation
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, relKey)
		if err != nil {
			return "", nil, fmt.Errorf("failed to create unique Key: %w", err)
		}
		id, err := space.DeriveObjectID(ctx, uk)
		if err != nil {
			return "", nil, fmt.Errorf("failed to derive object id: %w", err)
		}
		recommendedRelationIDs = append(recommendedRelationIDs, id)
	}
	object.Fields[bundle.RelationKeyId.String()] = pbtypes.String(id)
	object.Fields[bundle.RelationKeyLayout.String()] = pbtypes.Float64(float64(model.ObjectType_objectType))
	object.Fields[bundle.RelationKeyRecommendedLayout.String()] = pbtypes.Int64(rawRecommendedLayout)
	object.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(recommendedRelationIDs)

	if details.GetFields() == nil {
		details.Fields = map[string]*types.Value{}
	}

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

func (s *service) installTemplatesForObjectType(spc space.Space, typeKey domain.TypeKey) error {
	bundledTemplates, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(bundle.TypeKeyTemplate.BundledURL()),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(typeKey.BundledURL()),
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
		id := pbtypes.GetString(record.Details, bundle.RelationKeyId.String())
		if _, exists := installedTemplatesIDs[id]; exists {
			continue
		}

		_, err := s.blockService.TemplateCloneInSpace(spc, id)
		if err != nil {
			return fmt.Errorf("clone template: %w", err)
		}
	}
	return nil
}

func (s *service) listInstalledTemplatesForType(spc space.Space, typeKey domain.TypeKey) (map[string]struct{}, error) {
	templateTypeID, err := spc.GetTypeIdByKey(context.Background(), bundle.TypeKeyTemplate)
	if err != nil {
		return nil, fmt.Errorf("get template type id by key: %w", err)
	}
	targetObjectTypeID, err := spc.GetTypeIdByKey(context.Background(), typeKey)
	if err != nil {
		return nil, fmt.Errorf("get type id by key: %w", err)
	}
	alreadyInstalledTemplates, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(templateTypeID),
			},
			{
				RelationKey: bundle.RelationKeyTargetObjectType.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(targetObjectTypeID),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spc.Id()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	existingTemplatesMap := map[string]struct{}{}
	for _, rec := range alreadyInstalledTemplates {
		sourceObject := pbtypes.GetString(rec.Details, bundle.RelationKeySourceObject.String())
		if sourceObject != "" {
			existingTemplatesMap[sourceObject] = struct{}{}
		}
	}
	return existingTemplatesMap, nil
}
