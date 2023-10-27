package objectcreator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (s *service) InstallBundledObjects(
	ctx context.Context,
	space space.Space,
	sourceObjectIds []string,
) (ids []string, objects []*types.Struct, err error) {
	ids, objects, err = s.reinstallBundledObjects(space, sourceObjectIds)
	if err != nil {
		return nil, nil, fmt.Errorf("reinstall bundled objects: %w", err)
	}

	existingObjectMap, err := s.listInstalledObjects(space, sourceObjectIds)
	if err != nil {
		return nil, nil, fmt.Errorf("list installed objects: %w", err)
	}

	marketplaceSpace, err := s.spaceService.Get(ctx, addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return nil, nil, fmt.Errorf("get marketplace space: %w", err)
	}

	for _, sourceObjectId := range sourceObjectIds {
		if _, ok := existingObjectMap[sourceObjectId]; ok {
			continue
		}
		installingDetails, err := s.prepareDetailsForInstallingObject(ctx, marketplaceSpace, sourceObjectId, space)
		if err != nil {
			return nil, nil, fmt.Errorf("prepare details for installing object: %w", err)
		}
		id, newDetails, err := s.installObject(ctx, space, installingDetails)
		if err != nil {
			return nil, nil, fmt.Errorf("install object: %w", err)
		}
		if id != "" && newDetails != nil {
			ids = append(ids, id)
			objects = append(objects, newDetails)
		}
	}

	return
}

func (s *service) installObject(ctx context.Context, space space.Space, installingDetails *types.Struct) (id string, newDetails *types.Struct, err error) {
	uk, err := domain.UnmarshalUniqueKey(pbtypes.GetString(installingDetails, bundle.RelationKeyUniqueKey.String()))
	if err != nil {
		return "", nil, fmt.Errorf("unmarshal unique key: %w", err)
	}
	var objectTypeKey domain.TypeKey
	if uk.SmartblockType() == coresb.SmartBlockTypeRelation {
		objectTypeKey = bundle.TypeKeyRelation
	} else if uk.SmartblockType() == coresb.SmartBlockTypeObjectType {
		objectTypeKey = bundle.TypeKeyObjectType
	} else {
		return "", nil, fmt.Errorf("unsupported object type: %s", uk.SmartblockType())
	}

	id, newDetails, err = s.CreateObjectInSpace(ctx, space, CreateObjectRequest{
		Details:       installingDetails,
		ObjectTypeKey: objectTypeKey,
	})
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		// we don't want to stop adding other objects
		log.Errorf("error while block create: %v", err)
		return "", nil, nil
	}
	return id, newDetails, nil
}

func (s *service) listInstalledObjects(space space.Space, sourceObjectIds []string) (map[string]struct{}, error) {
	existingObjects, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySourceObject.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(sourceObjectIds),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(space.Id()),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query existing objects: %w", err)
	}
	existingObjectMap := make(map[string]struct{}, len(existingObjects))
	for _, existingObject := range existingObjects {
		existingObjectMap[pbtypes.GetString(existingObject.Details, bundle.RelationKeySourceObject.String())] = struct{}{}
	}
	return existingObjectMap, nil
}

func (s *service) reinstallBundledObjects(space space.Space, sourceObjectIDs []string) ([]string, []*types.Struct, error) {
	uninstalledObjects, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySourceObject.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(sourceObjectIDs),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(space.Id()),
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
		var typeKey domain.TypeKey
		err = space.Do(id, func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			st.SetDetailAndBundledRelation(bundle.RelationKeyIsUninstalled, pbtypes.Bool(false))
			st.SetDetailAndBundledRelation(bundle.RelationKeyIsDeleted, pbtypes.Bool(false))
			typeKey = domain.TypeKey(st.UniqueKeyInternal())

			ids = append(ids, id)
			objects = append(objects, st.CombinedDetails())

			return sb.Apply(st)
		})
		if err != nil {
			sourceObjectID := pbtypes.GetString(rec.Details, bundle.RelationKeySourceObject.String())
			return nil, nil, fmt.Errorf("reinstall object %s (source object: %s): %w", id, sourceObjectID, err)
		}

		err = s.installTemplatesForObjectType(space, typeKey)
		if err != nil {
			return nil, nil, fmt.Errorf("install templates for object type %s: %w", typeKey, err)
		}
	}

	return ids, objects, nil
}

func (s *service) prepareDetailsForInstallingObject(ctx context.Context, sourceSpace space.Space, sourceObjectId string, spc space.Space) (*types.Struct, error) {
	var details *types.Struct
	err := sourceSpace.Do(sourceObjectId, func(b smartblock.SmartBlock) error {
		details = b.CombinedDetails()
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get details from source space: %w", err)
	}

	spaceID := spc.Id()
	sourceId := pbtypes.GetString(details, bundle.RelationKeyId.String())
	details.Fields[bundle.RelationKeySpaceId.String()] = pbtypes.String(spaceID)
	details.Fields[bundle.RelationKeySourceObject.String()] = pbtypes.String(sourceId)
	details.Fields[bundle.RelationKeyIsReadonly.String()] = pbtypes.Bool(false)

	bundledRelationIds := pbtypes.GetStringList(details, bundle.RelationKeyRecommendedRelations.String())
	if len(bundledRelationIds) > 0 {
		recommendedRelationKeys := make([]string, 0, len(bundledRelationIds))
		for _, id := range bundledRelationIds {
			key, err := bundle.RelationKeyFromID(id)
			if err != nil {
				return nil, fmt.Errorf("relation key from id %s: %w", id, err)
			}
			recommendedRelationKeys = append(recommendedRelationKeys, key.String())
		}
		recommendedRelationIds, err := s.prepareRecommendedRelationIds(ctx, spc, recommendedRelationKeys)
		if err != nil {
			return nil, fmt.Errorf("prepare recommended relation ids: %w", err)
		}
		details.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(recommendedRelationIds)
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
			id, err := spc.DeriveObjectID(ctx, uniqueKey)
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
