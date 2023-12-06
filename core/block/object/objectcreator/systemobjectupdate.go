package objectcreator

import (
	"strings"

	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var revisionKey = bundle.RelationKeyRevision.String()

func (s *service) updateSystemObjects(space space.Space, objects map[string]*types.Struct) {
	marketRels, err := s.objectStore.ListAllRelations(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		log.Errorf("failed to get relations from marketplace space: %v", err)
		return
	}

	marketTypes, err := s.listAllObjectTypes(addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		log.Errorf("failed to get object types from marketplace space: %v", err)
		return
	}

	for source, details := range objects {
		if strings.HasPrefix(source, addr.BundledRelationURLPrefix) {
			updateSystemRelation(space, relationutils.RelationFromStruct(details), marketRels)
		} else if strings.HasPrefix(source, addr.BundledObjectTypeURLPrefix) {
			updateSystemObjectType(space, details, marketTypes)
		}
	}
}

func (s *service) listAllObjectTypes(spaceId string) (map[string]*types.Struct, error) {
	records, _, err := s.objectStore.Query(database.Query{
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

func updateSystemRelation(space space.Space, rel *relationutils.Relation, marketRels relationutils.Relations) {
	marketRel := marketRels.GetModelByKey(rel.Key)
	if marketRel == nil || !lo.Contains(bundle.SystemRelations, domain.RelationKey(rel.Key)) || marketRel.Revision <= rel.Revision {
		return
	}
	details := buildRelationDiffDetails(marketRel, rel.Relation)
	if len(details) != 0 {
		if err := space.Do(rel.Id, func(sb smartblock.SmartBlock) error {
			if ds, ok := sb.(basic.DetailsSettable); ok {
				return ds.SetDetails(nil, details, false)
			}
			return nil
		}); err != nil {
			log.Errorf("failed to update system relation %s in space %s: %v", rel.Key, space.Id(), err)
		}
	}
}

func updateSystemObjectType(space space.Space, objectType *types.Struct, marketTypes map[string]*types.Struct) {
	marketType, found := marketTypes[pbtypes.GetString(objectType, bundle.RelationKeySourceObject.String())]
	rawKey := pbtypes.GetString(objectType, bundle.RelationKeyUniqueKey.String())
	uk, err := domain.UnmarshalUniqueKey(rawKey)
	if !found || err != nil || !lo.Contains(bundle.SystemTypes, domain.TypeKey(uk.InternalKey())) ||
		pbtypes.GetInt64(marketType, revisionKey) <= pbtypes.GetInt64(objectType, revisionKey) {
		return
	}
	details := buildTypeDiffDetails(marketType, objectType)
	if len(details) != 0 {
		if err = space.Do(pbtypes.GetString(objectType, bundle.RelationKeyId.String()), func(sb smartblock.SmartBlock) error {
			if ds, ok := sb.(basic.DetailsSettable); ok {
				return ds.SetDetails(nil, details, false)
			}
			return nil
		}); err != nil {
			log.Errorf("failed to update system type %s in space %s: %v", uk.InternalKey(), space.Id(), err)
		}
	}
}

func buildRelationDiffDetails(origin, current *model.Relation) (details []*pb.RpcObjectSetDetailsDetail) {
	details = []*pb.RpcObjectSetDetailsDetail{{
		Key:   bundle.RelationKeyRevision.String(),
		Value: pbtypes.Int64(origin.Revision),
	}}

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

	if origin.ReadOnly != current.ReadOnly {
		details = append(details, &pb.RpcObjectSetDetailsDetail{
			Key:   bundle.RelationKeyIsReadonly.String(),
			Value: pbtypes.Bool(origin.ReadOnly),
		})
	}

	return
}

func buildTypeDiffDetails(origin, current *types.Struct) (details []*pb.RpcObjectSetDetailsDetail) {
	diff := pbtypes.StructDiff(current, origin)
	diff = pbtypes.StructFilterKeys(diff, []string{
		bundle.RelationKeyName.String(), bundle.RelationKeyDescription.String(),
		bundle.RelationKeyIsHidden.String(), bundle.RelationKeyRevision.String(),
	})

	for key, value := range diff.Fields {
		details = append(details, &pb.RpcObjectSetDetailsDetail{Key: key, Value: value})
	}

	return
}
