package installer

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/lastused"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "bundle-objects-installer"

var log = logging.Logger(CName)

type objectArchiver interface {
	SetIsArchived(objectId string, isArchived bool) error
}

type BundleObjectInstaller interface {
	InstallBundledObjects(ctx context.Context, space clientspace.Space, sourceObjectIds []string, isNewSpace bool) (ids []string, objects []*types.Struct, err error)
	app.Component
}

type objectInstaller struct {
	spaceService   space.Service
	objectStore    objectstore.ObjectStore
	archiver       objectArchiver
	creatorService objectcreator.Service
}

func New() BundleObjectInstaller {
	return &objectInstaller{}
}

func (i *objectInstaller) Init(a *app.App) error {
	i.spaceService = app.MustComponent[space.Service](a)
	i.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	i.archiver = app.MustComponent[objectArchiver](a)
	i.creatorService = app.MustComponent[objectcreator.Service](a)
	return nil
}

func (i *objectInstaller) Name() string {
	return CName
}

func (i *objectInstaller) BundledObjectsIdsToInstall(
	ctx context.Context,
	space clientspace.Space,
	sourceObjectIds []string,
) (ids domain.BundledObjectIds, err error) {
	marketplaceSpace, err := i.spaceService.Get(ctx, addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return nil, fmt.Errorf("get marketplace space: %w", err)
	}

	existingObjectMap, err := i.listInstalledObjects(space, sourceObjectIds)
	if err != nil {
		return nil, fmt.Errorf("list installed objects: %w", err)
	}

	for _, sourceObjectId := range sourceObjectIds {
		if _, ok := existingObjectMap[sourceObjectId]; ok {
			continue
		}

		err = marketplaceSpace.Do(sourceObjectId, func(b smartblock.SmartBlock) error {
			uk, err := domain.UnmarshalUniqueKey(pbtypes.GetString(b.CombinedDetails(), bundle.RelationKeyUniqueKey.String()))
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

func (i *objectInstaller) InstallBundledObjects(
	ctx context.Context,
	space clientspace.Space,
	sourceObjectIds []string,
	isNewSpace bool,
) (ids []string, objects []*types.Struct, err error) {
	if space.IsReadOnly() {
		return
	}

	marketplaceSpace, err := i.spaceService.Get(ctx, addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return nil, nil, fmt.Errorf("get marketplace space: %w", err)
	}

	ids, objects, err = i.reinstallBundledObjects(ctx, marketplaceSpace, space, sourceObjectIds)
	if err != nil {
		return nil, nil, fmt.Errorf("reinstall bundled objects: %w", err)
	}

	existingObjectMap, err := i.listInstalledObjects(space, sourceObjectIds)
	if err != nil {
		return nil, nil, fmt.Errorf("list installed objects: %w", err)
	}

	for _, sourceObjectId := range sourceObjectIds {
		if _, ok := existingObjectMap[sourceObjectId]; ok {
			continue
		}
		installingDetails, err := i.prepareDetailsForInstallingObject(ctx, marketplaceSpace, space, sourceObjectId, isNewSpace)
		if err != nil {
			return nil, nil, fmt.Errorf("prepare details for installing object: %w", err)
		}
		id, newDetails, err := i.installObject(ctx, space, installingDetails)
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

func (i *objectInstaller) installObject(ctx context.Context, space clientspace.Space, installingDetails *types.Struct) (id string, newDetails *types.Struct, err error) {
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

	id, newDetails, err = i.creatorService.CreateObjectInSpace(ctx, space, objectcreator.CreateObjectRequest{
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

func (i *objectInstaller) listInstalledObjects(space clientspace.Space, sourceObjectIds []string) (map[string]*types.Struct, error) {
	existingObjects, err := i.objectStore.SpaceIndex(space.Id()).Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySourceObject.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(sourceObjectIds),
			},
			{
				Operator: model.BlockContentDataviewFilter_Or,
				NestedFilters: []*model.BlockContentDataviewFilter{
					{
						RelationKey: bundle.RelationKeyLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
					},
					{
						RelationKey: bundle.RelationKeyLayout.String(),
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
					},
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query existing objects: %w", err)
	}
	existingObjectMap := make(map[string]*types.Struct, len(existingObjects))
	for _, existingObject := range existingObjects {
		existingObjectMap[pbtypes.GetString(existingObject.Details, bundle.RelationKeySourceObject.String())] = existingObject.Details
	}
	return existingObjectMap, nil
}

func (i *objectInstaller) reinstallBundledObjects(
	ctx context.Context, sourceSpace, space clientspace.Space, sourceObjectIDs []string,
) (ids []string, objects []*types.Struct, err error) {
	deletedObjects, err := i.queryDeletedObjects(space, sourceObjectIDs)
	if err != nil {
		return nil, nil, fmt.Errorf("query deleted objects: %w", err)
	}

	for _, rec := range deletedObjects {
		id, typeKey, details, err := i.reinstallObject(ctx, sourceSpace, space, rec.Details)
		if err != nil {
			return nil, nil, err
		}

		ids = append(ids, id)
		objects = append(objects, details)

		if err = i.creatorService.CreateTemplatesForObjectType(space, typeKey); err != nil {
			return nil, nil, fmt.Errorf("install templates for object type %s: %w", typeKey, err)
		}
	}

	return ids, objects, nil
}

func (i *objectInstaller) reinstallObject(
	ctx context.Context, sourceSpace, space clientspace.Space, currentDetails *types.Struct,
) (id string, key domain.TypeKey, details *types.Struct, err error) {
	id = pbtypes.GetString(currentDetails, bundle.RelationKeyId.String())
	var (
		sourceObjectId = pbtypes.GetString(currentDetails, bundle.RelationKeySourceObject.String())
		isArchived     = pbtypes.GetBool(currentDetails, bundle.RelationKeyIsArchived.String())
	)

	installingDetails, err := i.prepareDetailsForInstallingObject(ctx, sourceSpace, space, sourceObjectId, false)
	if err != nil {
		return "", "", nil, fmt.Errorf("prepare details for installing object: %w", err)
	}

	err = space.Do(id, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.SetDetails(installingDetails)
		st.SetDetailAndBundledRelation(bundle.RelationKeyIsUninstalled, pbtypes.Bool(false))
		st.SetDetailAndBundledRelation(bundle.RelationKeyIsDeleted, pbtypes.Bool(false))

		key = domain.TypeKey(st.UniqueKeyInternal())
		details = st.CombinedDetails()

		return sb.Apply(st)
	})
	if err != nil {
		return "", "", nil, fmt.Errorf("reinstall object %s (source object: %s): %w", id, sourceObjectId, err)
	}

	if isArchived {
		// we should do archive operations only via Archive object
		if err = i.archiver.SetIsArchived(id, false); err != nil {
			return "", "", nil, fmt.Errorf("failed to restore object %s (source object: %s) from bin: %w", id, sourceObjectId, err)
		}
	}

	return id, key, details, nil
}

func (i *objectInstaller) prepareDetailsForInstallingObject(
	ctx context.Context,
	sourceSpace, spc clientspace.Space,
	sourceObjectId string,
	isNewSpace bool,
) (*types.Struct, error) {
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

	// we should delete old createdDate as it belongs to source object from marketplace
	delete(details.Fields, bundle.RelationKeyCreatedDate.String())

	if isNewSpace {
		lastused.SetLastUsedDateForInitialObjectType(sourceId, details)
	}

	uk, err := domain.UnmarshalUniqueKey(pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String()))
	if err != nil {
		return nil, fmt.Errorf("unmarshal unique key: %w", err)
	}

	switch uk.SmartblockType() {
	case coresb.SmartBlockTypeBundledObjectType, coresb.SmartBlockTypeObjectType:
		relationKeys, isAlreadyFilled, err := objectcreator.FillRecommendedRelations(ctx, spc, details)
		if err != nil {
			return nil, fmt.Errorf("fill recommended relations: %w", err)
		}

		if !isAlreadyFilled {
			bundledRelationIds := make([]string, len(relationKeys))
			for j, key := range relationKeys {
				bundledRelationIds[j] = key.BundledURL()
			}
			if _, _, err = i.InstallBundledObjects(ctx, spc, bundledRelationIds, isNewSpace); err != nil {
				return nil, fmt.Errorf("install recommended relations: %w", err)
			}
		}

	case coresb.SmartBlockTypeBundledRelation, coresb.SmartBlockTypeRelation:
		if err = objectcreator.FillRelationFormatObjectTypes(ctx, spc, details); err != nil {
			return nil, fmt.Errorf("fill relation format objectTypes: %w", err)
		}
	}

	return details, nil
}

func (i *objectInstaller) queryDeletedObjects(space clientspace.Space, sourceObjectIDs []string) ([]database.Record, error) {
	sourceList, err := pbtypes.ValueListWrapper(pbtypes.StringList(sourceObjectIDs))
	if err != nil {
		return nil, err
	}
	return i.objectStore.SpaceIndex(space.Id()).QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FiltersOr{
			database.FilterEq{
				Key:   bundle.RelationKeyLayout.String(),
				Value: pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
			database.FilterEq{
				Key:   bundle.RelationKeyLayout.String(),
				Value: pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		},
		database.FilterIn{
			Key:   bundle.RelationKeySourceObject.String(),
			Value: sourceList,
		},
		database.FiltersOr{
			database.FilterEq{
				Key:   bundle.RelationKeyIsDeleted.String(),
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: pbtypes.Bool(true),
			},
			database.FilterEq{
				Key:   bundle.RelationKeyIsArchived.String(),
				Cond:  model.BlockContentDataviewFilter_Equal,
				Value: pbtypes.Bool(true),
			},
		},
	}}, 0, 0)
}
