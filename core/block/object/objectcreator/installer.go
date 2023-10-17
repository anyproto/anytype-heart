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
	spaceID := space.Id()

	ids, objects, err = s.reinstallBundledObjects(space, sourceObjectIds)
	if err != nil {
		return nil, nil, fmt.Errorf("reinstall bundled objects: %w", err)
	}

	// todo: replace this func to the universal space to space copy
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
				Value:       pbtypes.String(spaceID),
			},
		},
	})
	var existingObjectMap = make(map[string]struct{})
	for _, existingObject := range existingObjects {
		existingObjectMap[pbtypes.GetString(existingObject.Details, bundle.RelationKeySourceObject.String())] = struct{}{}
	}

	marketplaceSpace, err := s.spaceService.Get(ctx, addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return nil, nil, fmt.Errorf("get marketplace space: %w", err)
	}

	for _, sourceObjectId := range sourceObjectIds {
		if _, ok := existingObjectMap[sourceObjectId]; ok {
			continue
		}
		err = marketplaceSpace.Do(sourceObjectId, func(b smartblock.SmartBlock) error {
			d, err := s.prepareDetailsForInstallingObject(ctx, space, b.CombinedDetails())
			if err != nil {
				return err
			}

			uk, err := domain.UnmarshalUniqueKey(pbtypes.GetString(d, bundle.RelationKeyUniqueKey.String()))
			if err != nil {
				return err
			}
			var objectTypeKey domain.TypeKey
			if uk.SmartblockType() == coresb.SmartBlockTypeRelation {
				objectTypeKey = bundle.TypeKeyRelation
			} else if uk.SmartblockType() == coresb.SmartBlockTypeObjectType {
				objectTypeKey = bundle.TypeKeyObjectType
			} else {
				return fmt.Errorf("unsupported object type: %s", b.Type())
			}

			id, object, err := s.CreateObjectInSpace(ctx, space, CreateObjectRequest{
				Details:       d,
				ObjectTypeKey: objectTypeKey,
			})
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
		err = space.Do(id, func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			st.SetDetailAndBundledRelation(bundle.RelationKeyIsUninstalled, pbtypes.Bool(false))
			st.SetDetailAndBundledRelation(bundle.RelationKeyIsDeleted, pbtypes.Bool(false))

			ids = append(ids, id)
			objects = append(objects, st.CombinedDetails())

			return sb.Apply(st)
		})
		if err != nil {
			sourceObjectID := pbtypes.GetString(rec.Details, bundle.RelationKeySourceObject.String())
			return nil, nil, fmt.Errorf("reinstall object %s (source object: %s): %w", id, sourceObjectID, err)
		}

	}

	return ids, objects, nil
}

func (s *service) prepareDetailsForInstallingObject(ctx context.Context, spc space.Space, details *types.Struct) (*types.Struct, error) {
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
