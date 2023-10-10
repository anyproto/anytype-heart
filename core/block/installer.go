package block

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *Service) InstallBundledObject(
	ctx context.Context,
	spaceID string,
	sourceObjectId string,
) (id string, object *types.Struct, err error) {
	ids, details, err := s.InstallBundledObjects(ctx, spaceID, []string{sourceObjectId})
	if err != nil {
		return "", nil, err
	}
	if len(ids) == 0 {
		return "", nil, fmt.Errorf("failed to add object")
	}

	return ids[0], details[0], nil
}

func (s *Service) prepareDetailsForInstallingObject(ctx context.Context, spaceID string, details *types.Struct) (*types.Struct, error) {
	sourceId := pbtypes.GetString(details, bundle.RelationKeyId.String())
	if pbtypes.GetString(details, bundle.RelationKeySpaceId.String()) != addr.AnytypeMarketplaceWorkspace {
		return nil, errors.New("object is not bundled")
	}
	details.Fields[bundle.RelationKeySpaceId.String()] = pbtypes.String(spaceID)

	details.Fields[bundle.RelationKeySourceObject.String()] = pbtypes.String(sourceId)
	details.Fields[bundle.RelationKeyIsReadonly.String()] = pbtypes.Bool(false)

	switch pbtypes.GetString(details, bundle.RelationKeyType.String()) {
	case bundle.TypeKeyObjectType.BundledURL():
		typeID := s.anytype.GetSystemTypeID(spaceID, bundle.TypeKeyObjectType)
		details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(typeID)
	case bundle.TypeKeyRelation.BundledURL():
		typeID := s.anytype.GetSystemTypeID(spaceID, bundle.TypeKeyRelation)
		details.Fields[bundle.RelationKeyType.String()] = pbtypes.String(typeID)
	default:
		return nil, fmt.Errorf("unknown object type: %s", pbtypes.GetString(details, bundle.RelationKeyType.String()))
	}
	relations := pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String())

	if len(relations) > 0 {
		for i, relation := range relations {
			// replace relation url with id
			uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, strings.TrimPrefix(relation, addr.BundledRelationURLPrefix))
			if err != nil {
				// should never happen
				return nil, err
			}
			id, err := s.objectCache.DeriveObjectID(ctx, spaceID, uniqueKey)
			if err != nil {
				// should never happen
				return nil, err
			}
			relations[i] = id
		}
		details.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(relations)
	}

	objectTypes := pbtypes.GetStringList(details, bundle.RelationKeyRelationFormatObjectTypes.String())

	if len(objectTypes) > 0 {
		for i, objectType := range objectTypes {
			// replace object type url with id
			uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, strings.TrimPrefix(objectType, addr.BundledObjectTypeURLPrefix))
			if err != nil {
				// should never happen
				return nil, err
			}
			id, err := s.objectCache.DeriveObjectID(ctx, spaceID, uniqueKey)
			if err != nil {
				// should never happen
				return nil, err
			}
			objectTypes[i] = id
		}
		details.Fields[bundle.RelationKeyRelationFormatObjectTypes.String()] = pbtypes.StringList(objectTypes)
	}

	return details, nil
}

func (s *Service) InstallBundledObjects(
	ctx context.Context,
	spaceID string,
	sourceObjectIds []string,
) (ids []string, objects []*types.Struct, err error) {
	ids, objects, err = s.reinstallBundledObjects(spaceID, sourceObjectIds)
	if err != nil {
		return nil, nil, fmt.Errorf("reinstall bundled objects: %w", err)
	}

	existingObjectMap, err := s.listInstalledBundledObjects(spaceID, sourceObjectIds)
	if err != nil {
		return nil, nil, fmt.Errorf("list existing bundled objects: %w", err)
	}

	for _, sourceObjectId := range sourceObjectIds {
		err = Do(s, sourceObjectId, func(b smartblock.SmartBlock) error {
			bundledDetails := b.CombinedDetails()
			rawUniqueKey := pbtypes.GetString(bundledDetails, bundle.RelationKeyUniqueKey.String())
			if _, exists := existingObjectMap[rawUniqueKey]; exists {
				return nil
			}

			d, err := s.prepareDetailsForInstallingObject(ctx, spaceID, bundledDetails)
			if err != nil {
				return err
			}

			uk, err := domain.UnmarshalUniqueKey(rawUniqueKey)
			if err != nil {
				return err
			}

			// create via the state directly, because we have cyclic dependencies and we want to avoid typeId resolving from the details
			st := state.NewDocWithUniqueKey("", nil, uk).(*state.State)
			st.SetDetails(d)

			var objectTypeKey domain.TypeKey
			if uk.SmartblockType() == coresb.SmartBlockTypeRelation {
				objectTypeKey = bundle.TypeKeyRelation
			} else if uk.SmartblockType() == coresb.SmartBlockTypeObjectType {
				objectTypeKey = bundle.TypeKeyObjectType
			} else {
				return fmt.Errorf("unsupported object type: %s", b.Type())
			}

			id, object, err := s.objectCreator.CreateSmartBlockFromState(
				ctx,
				spaceID,
				uk.SmartblockType(),
				[]domain.TypeKey{objectTypeKey},
				nil,
				st,
			)
			if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
				// we don't want to stop adding other objects
				log.Errorf("error while block create: %v", err)
				return nil
			}

			if uk.SmartblockType() == coresb.SmartBlockTypeObjectType {
				installingObjectTypeKey := domain.TypeKey(uk.InternalKey())
				err = s.installTemplatesForObjectType(spaceID, installingObjectTypeKey)
				if err != nil {
					log.With("spaceID", spaceID, "objectTypeKey", installingObjectTypeKey).Errorf("error while installing templates: %s", err)
				}
			}

			ids = append(ids, id)
			objects = append(objects, object)
			return nil
		})
		if err != nil {
			return
		}
	}

	return
}

func (s *Service) listInstalledBundledObjects(spaceId string, bundledIds []string) (map[string]struct{}, error) {
	rawUniqueKeys := convertBundledIdsToRawUniqueKeys(bundledIds)
	existingObjects, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(rawUniqueKeys),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
		},
	})
	if err != nil {
		return nil, err
	}
	existingObjectMap := make(map[string]struct{}, len(existingObjects))
	for _, existingObject := range existingObjects {
		existingObjectMap[pbtypes.GetString(existingObject.Details, bundle.RelationKeyUniqueKey.String())] = struct{}{}
	}
	return existingObjectMap, nil
}

func convertBundledIdsToRawUniqueKeys(bundledIds []string) []string {
	uniqueKeys := make([]string, 0, len(bundledIds))
	for _, id := range bundledIds {
		typeKey, err := bundle.TypeKeyFromUrl(id)
		if err == nil {
			uniqueKeys = append(uniqueKeys, domain.MustUniqueKey(coresb.SmartBlockTypeObjectType, typeKey.String()).Marshal())
			continue
		}
		relationKey, err := bundle.RelationKeyFromID(id)
		if err == nil {
			uniqueKeys = append(uniqueKeys, domain.MustUniqueKey(coresb.SmartBlockTypeRelation, relationKey.String()).Marshal())
			continue
		}
	}
	return uniqueKeys
}

func (s *Service) reinstallBundledObjects(spaceID string, bundledIds []string) ([]string, []*types.Struct, error) {
	rawUniqueKeys := convertBundledIdsToRawUniqueKeys(bundledIds)
	uninstalledObjects, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyUniqueKey.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(rawUniqueKeys),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceID),
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Bool(true),
			},
		},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("query uninstalled objects: %w", err)
	}

	var (
		ids     []string
		objects []*types.Struct
	)
	for _, rec := range uninstalledObjects {
		id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		err = Do(s, id, func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			st.SetDetailAndBundledRelation(bundle.RelationKeyIsUninstalled, pbtypes.Bool(false))
			st.SetDetailAndBundledRelation(bundle.RelationKeyIsDeleted, pbtypes.Bool(false))

			ids = append(ids, id)
			objects = append(objects, st.CombinedDetails())

			return sb.Apply(st)
		})
		if err != nil {
			rawUniqueKey := pbtypes.GetString(rec.Details, bundle.RelationKeyUniqueKey.String())
			return nil, nil, fmt.Errorf("reinstall object %s (unique key: %s): %w", id, rawUniqueKey, err)
		}

	}

	return ids, objects, nil
}

func (s *Service) installTemplatesForObjectType(spaceID string, typeKey domain.TypeKey) error {
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

	installedTemplatesIDs, err := s.listInstalledTemplatesForType(spaceID, typeKey)
	if err != nil {
		return fmt.Errorf("list installed templates: %w", err)
	}

	for _, record := range bundledTemplates {
		id := pbtypes.GetString(record.Details, bundle.RelationKeyId.String())
		if _, exists := installedTemplatesIDs[id]; exists {
			continue
		}

		_, err := s.TemplateClone(spaceID, id)
		if err != nil {
			return fmt.Errorf("clone template: %w", err)
		}
	}
	return nil
}

func (s *Service) listInstalledTemplatesForType(spaceID string, typeKey domain.TypeKey) (map[string]struct{}, error) {
	templateTypeID, err := s.systemObjectService.GetTypeIdByKey(context.Background(), spaceID, bundle.TypeKeyTemplate)
	if err != nil {
		return nil, fmt.Errorf("get template type id by key: %w", err)
	}
	targetObjectTypeID, err := s.systemObjectService.GetTypeIdByKey(context.Background(), spaceID, typeKey)
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
				Value:       pbtypes.String(spaceID),
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
