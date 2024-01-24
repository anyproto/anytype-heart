package objectcreator

import (
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var revisionKey = bundle.RelationKeyRevision.String()

func (s *service) reviseSystemObjects(space clientspace.Space, objects map[string]*types.Struct) {
	marketObjects, err := s.listAllTypesAndRelations(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		log.Errorf("failed to get relations from marketplace space: %v", err)
		return
	}

	for _, details := range objects {
		reviseSystemObject(space, details, marketObjects)

		fixReadonlyTagStatus(space, details)
	}
}

func (s *service) listAllTypesAndRelations(spaceId string) (map[string]*types.Struct, error) {
	records, _, err := s.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(model.ObjectType_objectType), int(model.ObjectType_relation)),
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

func reviseSystemObject(space clientspace.Space, localObject *types.Struct, marketObjects map[string]*types.Struct) {
	source := pbtypes.GetString(localObject, bundle.RelationKeySourceObject.String())
	marketObject, found := marketObjects[source]
	if !found || !isSystemObject(localObject) || pbtypes.GetInt64(marketObject, revisionKey) < pbtypes.GetInt64(localObject, revisionKey) {
		return
	}
	details := buildDiffDetails(marketObject, localObject)
	if len(details) != 0 {
		if err := space.Do(pbtypes.GetString(localObject, bundle.RelationKeyId.String()), func(sb smartblock.SmartBlock) error {
			if ds, ok := sb.(basic.DetailsSettable); ok {
				return ds.SetDetails(nil, details, false)
			}
			return nil
		}); err != nil {
			log.Errorf("failed to update system object %s in space %s: %v", source, space.Id(), err)
		}
	}
}

func isSystemObject(details *types.Struct) bool {
	rawKey := pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String())
	uk, err := domain.UnmarshalUniqueKey(rawKey)
	if err != nil {
		return false
	}
	switch uk.SmartblockType() {
	case coresb.SmartBlockTypeObjectType:
		return lo.Contains(bundle.SystemTypes, domain.TypeKey(uk.InternalKey()))
	case coresb.SmartBlockTypeRelation:
		return lo.Contains(bundle.SystemRelations, domain.RelationKey(uk.InternalKey()))
	}
	return false
}

func buildDiffDetails(origin, current *types.Struct) (details []*pb.RpcObjectSetDetailsDetail) {
	diff := pbtypes.StructDiff(current, origin)
	diff = pbtypes.StructFilterKeys(diff, []string{
		bundle.RelationKeyName.String(), bundle.RelationKeyDescription.String(),
		bundle.RelationKeyIsReadonly.String(), bundle.RelationKeyIsHidden.String(),
		bundle.RelationKeyRevision.String(), bundle.RelationKeyRelationReadonlyValue.String(),
		bundle.RelationKeyRelationMaxCount.String(),
	})

	for key, value := range diff.Fields {
		details = append(details, &pb.RpcObjectSetDetailsDetail{Key: key, Value: value})
	}
	return
}

func fixReadonlyTagStatus(space clientspace.Space, details *types.Struct) {
	format := model.RelationFormat(pbtypes.GetInt64(details, bundle.RelationKeyRelationFormat.String()))

	if format == model.RelationFormat_tag || format == model.RelationFormat_status {
		det := []*pb.RpcObjectSetDetailsDetail{{
			Key:   bundle.RelationKeyIsReadonly.String(),
			Value: pbtypes.Bool(false),
		}}
		if err := space.Do(pbtypes.GetString(details, bundle.RelationKeyId.String()), func(sb smartblock.SmartBlock) error {
			if ds, ok := sb.(basic.DetailsSettable); ok {
				return ds.SetDetails(nil, det, false)
			}
			return nil
		}); err != nil {
			source := pbtypes.GetString(details, bundle.RelationKeySourceObject.String())
			log.Errorf("failed to set readOnly=true to relation %s in space %s: %v", source, space.Id(), err)
		}
	}
}
