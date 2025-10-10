package objectcreator

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

func (s *service) BundledObjectsIdsToInstall(
	ctx context.Context,
	space clientspace.Space,
	sourceObjectIds []string,
) (ids domain.BundledObjectIds, err error) {
	marketplaceSpace, err := s.spaceService.Get(ctx, addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return nil, fmt.Errorf("get marketplace space: %w", err)
	}

	existingObjectMap, err := s.listInstalledObjects(space, sourceObjectIds)
	if err != nil {
		return nil, fmt.Errorf("list installed objects: %w", err)
	}

	for _, sourceObjectId := range sourceObjectIds {
		if _, ok := existingObjectMap[sourceObjectId]; ok {
			continue
		}

		err = marketplaceSpace.Do(sourceObjectId, func(b smartblock.SmartBlock) error {
			uk, err := domain.UnmarshalUniqueKey(b.CombinedDetails().GetString(bundle.RelationKeyUniqueKey))
			if err != nil {
				return err
			}
			objectId, err := space.DeriveObjectID(ctx, uk)
			if err != nil {
				return err
			}
			ids = append(ids, domain.BundledObjectId{
				SourceId:        sourceObjectId,
				DerivedObjectId: objectId,
			})
			return nil
		})
		if err != nil {
			return
		}
	}
	return
}

func (s *service) InstallBundledObjects(
	ctx context.Context,
	space clientspace.Space,
	sourceObjectIds []string,
) (ids []string, objects []*domain.Details, err error) {
	if space.IsReadOnly() {
		return
	}

	marketplaceSpace, err := s.spaceService.Get(ctx, addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return nil, nil, fmt.Errorf("get marketplace space: %w", err)
	}

	ids, objects, err = s.reinstallBundledObjects(ctx, marketplaceSpace, space, sourceObjectIds)
	if err != nil {
		return nil, nil, fmt.Errorf("reinstall bundled objects: %w", err)
	}

	existingObjectMap, err := s.listInstalledObjects(space, sourceObjectIds)
	if err != nil {
		return nil, nil, fmt.Errorf("list installed objects: %w", err)
	}

	for _, sourceObjectId := range sourceObjectIds {
		if _, ok := existingObjectMap[sourceObjectId]; ok {
			continue
		}
		installingDetails, err := s.prepareDetailsForInstallingObject(ctx, marketplaceSpace, space, sourceObjectId)
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

func (s *service) installObject(ctx context.Context, space clientspace.Space, installingDetails *domain.Details) (id string, newDetails *domain.Details, err error) {
	uk, err := domain.UnmarshalUniqueKey(installingDetails.GetString(bundle.RelationKeyUniqueKey))
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

	id, newDetails, err = s.createObjectInSpace(ctx, space, CreateObjectRequest{
		Details:       installingDetails,
		ObjectTypeKey: objectTypeKey,
	})
	log.Desugar().Info("install new object", zap.String("id", id))
	if err != nil && !errors.Is(err, treestorage.ErrTreeExists) {
		// we don't want to stop adding other objects
		log.Errorf("error while block create: %v", err)
		return "", nil, nil
	}
	return id, newDetails, nil
}

func (s *service) listInstalledObjects(space clientspace.Space, sourceObjectIds []string) (map[string]*domain.Details, error) {
	existingObjects, err := s.objectStore.SpaceIndex(space.Id()).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeySourceObject,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.StringList(sourceObjectIds),
			},
			{
				Operator: model.BlockContentDataviewFilter_Or,
				NestedFilters: []database.FilterRequest{
					{
						RelationKey: bundle.RelationKeyResolvedLayout,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.Int64(model.ObjectType_objectType),
					},
					{
						RelationKey: bundle.RelationKeyResolvedLayout,
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       domain.Int64(model.ObjectType_relation),
					},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query existing objects: %w", err)
	}
	existingObjectMap := make(map[string]*domain.Details, len(existingObjects))
	for _, existingObject := range existingObjects {
		existingObjectMap[existingObject.Details.GetString(bundle.RelationKeySourceObject)] = existingObject.Details
	}
	return existingObjectMap, nil
}

func (s *service) reinstallBundledObjects(
	ctx context.Context, sourceSpace, space clientspace.Space, sourceObjectIDs []string,
) (ids []string, objects []*domain.Details, err error) {
	deletedObjects, err := s.queryDeletedObjects(space, sourceObjectIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("query deleted objects: %w", err)
	}

	for _, rec := range deletedObjects {
		id, typeKey, details, err := s.reinstallObject(ctx, sourceSpace, space, rec.Details)
		if err != nil {
			return nil, nil, err
		}

		ids = append(ids, id)
		objects = append(objects, details)

		if err = s.createTemplatesForObjectType(space, typeKey); err != nil {
			return nil, nil, fmt.Errorf("install templates for object type %s: %w", typeKey, err)
		}
	}

	return ids, objects, nil
}

func (s *service) reinstallObject(
	ctx context.Context, sourceSpace, space clientspace.Space, currentDetails *domain.Details,
) (id string, key domain.TypeKey, details *domain.Details, err error) {
	id = currentDetails.GetString(bundle.RelationKeyId)
	var (
		sourceObjectId = currentDetails.GetString(bundle.RelationKeySourceObject)
		isArchived     = currentDetails.GetBool(bundle.RelationKeyIsArchived)
	)

	installingDetails, err := s.prepareDetailsForInstallingObject(ctx, sourceSpace, space, sourceObjectId)
	if err != nil {
		return "", "", nil, fmt.Errorf("prepare details for installing object: %w", err)
	}

	err = space.Do(id, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.SetDetails(installingDetails)
		st.SetDetailAndBundledRelation(bundle.RelationKeyIsUninstalled, domain.Bool(false))
		st.SetDetailAndBundledRelation(bundle.RelationKeyIsDeleted, domain.Bool(false))
		st.SetOriginalCreatedTimestamp(time.Now().Unix())

		key = domain.TypeKey(st.UniqueKeyInternal())
		details = st.CombinedDetails()

		return sb.Apply(st)
	})
	if err != nil {
		return "", "", nil, fmt.Errorf("reinstall object %s (source object: %s): %w", id, sourceObjectId, err)
	}

	if isArchived {
		// we should do archive operations only via Archive object
		if err = s.archiver.SetIsArchived(id, false); err != nil {
			return "", "", nil, fmt.Errorf("failed to restore object %s (source object: %s) from bin: %w", id, sourceObjectId, err)
		}
	}

	return id, key, details, nil
}

func (s *service) prepareDetailsForInstallingObject(
	ctx context.Context,
	sourceSpace, spc clientspace.Space,
	sourceObjectId string,
) (*domain.Details, error) {
	var details *domain.Details
	err := sourceSpace.Do(sourceObjectId, func(b smartblock.SmartBlock) error {
		details = b.CombinedDetails()
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get details from source space: %w", err)
	}

	spaceID := spc.Id()
	sourceId := details.GetString(bundle.RelationKeyId)
	details.SetString(bundle.RelationKeySpaceId, spaceID)
	details.SetString(bundle.RelationKeySourceObject, sourceId)
	details.SetBool(bundle.RelationKeyIsReadonly, false)
	details.SetInt64(bundle.RelationKeyCreatedDate, time.Now().Unix())

	uk, err := domain.UnmarshalUniqueKey(details.GetString(bundle.RelationKeyUniqueKey))
	if err != nil {
		return nil, fmt.Errorf("unmarshal unique key: %w", err)
	}

	switch uk.SmartblockType() {
	case coresb.SmartBlockTypeBundledObjectType, coresb.SmartBlockTypeObjectType:
		relationKeys, isAlreadyFilled, err := relationutils.FillRecommendedRelations(ctx, spc, details, domain.TypeKey(uk.InternalKey()))
		if err != nil {
			return nil, fmt.Errorf("fill recommended relations: %w", err)
		}

		if !isAlreadyFilled {
			bundledRelationIds := make([]string, len(relationKeys))
			for j, key := range relationKeys {
				bundledRelationIds[j] = key.BundledURL()
			}
			if _, _, err = s.InstallBundledObjects(ctx, spc, bundledRelationIds); err != nil {
				return nil, fmt.Errorf("install recommended relations: %w", err)
			}
		}

	case coresb.SmartBlockTypeBundledRelation, coresb.SmartBlockTypeRelation:
		if err = fillRelationFormatObjectTypes(ctx, spc, details); err != nil {
			return nil, fmt.Errorf("fill relation format objectTypes: %w", err)
		}
	}

	return details, nil
}

func (s *service) queryDeletedObjects(space clientspace.Space, sourceObjectIDs []string) ([]database.Record, error) {
	sourceList := make([]domain.Value, 0, len(sourceObjectIDs))
	for _, id := range sourceObjectIDs {
		sourceList = append(sourceList, domain.String(id))
	}

	return s.objectStore.SpaceIndex(space.Id()).QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FiltersOr{
			database.FilterEq{
				Key:   bundle.RelationKeyResolvedLayout,
				Value: domain.Int64(model.ObjectType_objectType),
			},
			database.FilterEq{
				Key:   bundle.RelationKeyResolvedLayout,
				Value: domain.Int64(model.ObjectType_relation),
			},
		},
		database.FilterIn{
			Key:   bundle.RelationKeySourceObject,
			Value: sourceList,
		},
		database.FiltersOr{
			database.FilterEq{
				Key:   bundle.RelationKeyIsDeleted,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: domain.Bool(true),
			},
			database.FilterEq{
				Key:   bundle.RelationKeyIsArchived,
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: domain.Bool(true),
			},
		},
	}}, 0, 0)
}
