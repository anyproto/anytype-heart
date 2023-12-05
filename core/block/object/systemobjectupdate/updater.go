package systemobjectupdate

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const CName = "systemobjectupdater"

var log = logging.Logger("system-objects-updater")

type SystemObjectUpdater struct {
	app.ComponentRunnable

	store   objectstore.ObjectStore
	storage storage.ClientStorage
	picker  getblock.ObjectGetter
}

func New() *SystemObjectUpdater {
	return &SystemObjectUpdater{}
}

func (u *SystemObjectUpdater) Init(a *app.App) error {
	u.store = a.MustComponent(objectstore.CName).(objectstore.ObjectStore)
	u.storage = a.MustComponent(spacestorage.CName).(storage.ClientStorage)
	u.picker = app.MustComponent[getblock.ObjectGetter](a)
	return nil
}

func (u *SystemObjectUpdater) Name() string {
	return CName
}

func (u *SystemObjectUpdater) Run(_ context.Context) error {
	go u.updateSystemObjects()
	return nil
}

func (u *SystemObjectUpdater) Close(_ context.Context) error {
	return nil
}

func (u *SystemObjectUpdater) updateSystemObjects() {
	marketRels, err := u.store.ListAllRelations(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		log.Errorf("failed to get relations from marketplace space: %v", err)
		return
	}

	marketTypes, err := u.listAllObjectTypes(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		log.Errorf("failed to get object types from marketplace space: %v", err)
		return
	}

	spaceIds, err := u.storage.AllSpaceIds()
	if err != nil {
		log.Errorf("failed to get spaces ids from the storage: %v", err)
		return
	}

	for _, spaceId := range spaceIds {
		u.updateSystemRelations(spaceId, marketRels)
		u.updateSystemObjectTypes(spaceId, marketTypes)
	}
}

func (u *SystemObjectUpdater) listAllObjectTypes(spaceId string) (map[string]*types.Struct, error) {
	records, _, err := u.store.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Float64(float64(model.ObjectType_objectType)),
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

	details := make(map[string]*types.Struct, len(records))
	for _, rec := range records {
		id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		details[id] = rec.Details
	}
	return details, nil
}

func (u *SystemObjectUpdater) updateSystemRelations(spaceId string, marketRels relationutils.Relations) {
	rels, err := u.store.ListAllRelations(spaceId)
	if err != nil {
		log.Errorf("failed to get relations for space %s: %v", spaceId, err)
		return
	}

	for _, rel := range rels.Models() {
		marketRel := marketRels.GetModelByKey(rel.Key)
		if marketRel == nil || !lo.Contains(bundle.SystemRelations, domain.RelationKey(rel.Key)) {
			continue
		}
		details := buildRelationDiffDetails(marketRel, rel)
		if len(details) != 0 {
			if err = getblock.Do(u.picker, rel.Id, func(sb basic.DetailsSettable) error {
				return sb.SetDetails(nil, details, false)
			}); err != nil {
				log.Errorf("failed to update system relation %s in space %s: %v", rel.Key, spaceId, err)
			}
		}
	}
}

func (u *SystemObjectUpdater) updateSystemObjectTypes(spaceId string, marketTypes map[string]*types.Struct) {
	objectTypes, err := u.listAllObjectTypes(spaceId)
	if err != nil {
		log.Errorf("failed to get object types for space %s: %v", spaceId, err)
		return
	}

	for id, objectType := range objectTypes {
		marketType, found := marketTypes[pbtypes.GetString(objectType, bundle.RelationKeySourceObject.String())]
		rawKey := pbtypes.GetString(objectType, bundle.RelationKeyUniqueKey.String())
		uk, err := domain.UnmarshalUniqueKey(rawKey)
		if !found || err != nil || !lo.Contains(bundle.SystemTypes, domain.TypeKey(uk.InternalKey())) {
			continue
		}
		details := buildTypeDiffDetails(marketType, objectType)
		if len(details) != 0 {
			if err = getblock.Do(u.picker, id, func(sb basic.DetailsSettable) error {
				return sb.SetDetails(nil, details, false)
			}); err != nil {
				log.Errorf("failed to update system type %s in space %s: %v", uk.InternalKey(), spaceId, err)
			}
		}
	}
}

func buildRelationDiffDetails(origin, current *model.Relation) (details []*pb.RpcObjectSetDetailsDetail) {
	if origin.Name != current.Name {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyName.String(),
			Value: pbtypes.String(origin.Name),
		})
	}

	if origin.Description != current.Description {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyDescription.String(),
			Value: pbtypes.String(origin.Description),
		})
	}

	if origin.Hidden != current.Hidden {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyIsHidden.String(),
			Value: pbtypes.Bool(origin.Hidden),
		})
	}

	return
}

func buildTypeDiffDetails(origin, current *types.Struct) (details []*pb.RpcObjectSetDetailsDetail) {
	diff := pbtypes.StructDiff(current, origin)
	diff = pbtypes.StructFilterKeys(diff, []string{
		bundle.RelationKeyName.String(), bundle.RelationKeyDescription.String(), bundle.RelationKeyIsHidden.String(),
	})

	for key, value := range diff.Fields {
		details = append(details, &pb.RpcObjectSetDetailsDetail{Key: key, Value: value})
	}

	return
}
