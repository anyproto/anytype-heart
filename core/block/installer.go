package block

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
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

type createRequest struct {
	details *types.Struct
}

func (r createRequest) GetDetails() *types.Struct {
	return r.details
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
			// CombinedDetails returns copy of details, so we can use it safely
			bundledDetails := b.CombinedDetails()
			rawUniqueKey := pbtypes.GetString(bundledDetails, bundle.RelationKeyUniqueKey.String())
			uniqueKey, err := domain.UnmarshalUniqueKey(rawUniqueKey)
			if err != nil {
				return err
			}
			if _, exists := existingObjectMap[rawUniqueKey]; exists {
				return nil
			}

			details, err := s.prepareDetailsForInstallingObject(ctx, spaceID, bundledDetails)
			if err != nil {
				return err
			}

			var objectTypeKey domain.TypeKey
			if uniqueKey.SmartblockType() == coresb.SmartBlockTypeRelation {
				objectTypeKey = bundle.TypeKeyRelation
			} else if uniqueKey.SmartblockType() == coresb.SmartBlockTypeObjectType {
				objectTypeKey = bundle.TypeKeyObjectType
			} else {
				return fmt.Errorf("unsupported object type: %s", b.Type())
			}

			req := objectcreator.CreateObjectRequest{
				Details:       details,
				ObjectTypeKey: objectTypeKey,
			}
			id, object, err := s.objectCreator.CreateObject(ctx, spaceID, req)
			if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
				// we don't want to stop adding other objects
				log.Errorf("error while block create: %v", err)
				return nil
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
