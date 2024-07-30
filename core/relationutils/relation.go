package relationutils

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func RelationFromDetails(det *domain.Details) *Relation {
	key := det.GetStringOrDefault(bundle.RelationKeyRelationKey, "")
	maxCount := int32(det.GetInt64OrDefault(bundle.RelationKeyRelationMaxCount, 0))
	return &Relation{
		Relation: &model.Relation{
			Id:               det.GetStringOrDefault(bundle.RelationKeyId, ""),
			Key:              key,
			Format:           model.RelationFormat(det.GetFloatOrDefault(bundle.RelationKeyRelationFormat, 0)),
			Name:             det.GetStringOrDefault(bundle.RelationKeyName, ""),
			DataSource:       model.Relation_details,
			Hidden:           det.GetBoolOrDefault(bundle.RelationKeyIsHidden, false),
			ReadOnly:         det.GetBoolOrDefault(bundle.RelationKeyRelationReadonlyValue, false),
			ReadOnlyRelation: false,
			Multi:            maxCount > 1,
			ObjectTypes:      det.GetStringListOrDefault(bundle.RelationKeyRelationFormatObjectTypes, nil),
			MaxCount:         maxCount,
			Description:      det.GetStringOrDefault(bundle.RelationKeyDescription, ""),
			Scope:            model.RelationScope(det.GetFloatOrDefault(bundle.RelationKeyScope, 0)),
			Creator:          det.GetStringOrDefault(bundle.RelationKeyCreator, ""),
			Revision:         int64(det.GetInt64OrDefault(bundle.RelationKeyRevision, 0)),
		},
	}

}

type Relation struct {
	*model.Relation
}

func (r *Relation) RelationLink() *model.RelationLink {
	return &model.RelationLink{
		Format: r.Format,
		Key:    r.Key,
	}
}

func (r *Relation) ToStruct() *types.Struct {
	return &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyCreator.String():                   pbtypes.String(r.GetCreator()),
			bundle.RelationKeyDescription.String():               pbtypes.String(r.GetDescription()),
			bundle.RelationKeyId.String():                        pbtypes.String(r.Id),
			bundle.RelationKeyIsHidden.String():                  pbtypes.Bool(r.GetHidden()),
			bundle.RelationKeyIsReadonly.String():                pbtypes.Bool(r.GetReadOnlyRelation()),
			bundle.RelationKeyLayout.String():                    pbtypes.Int64(int64(model.ObjectType_relation)),
			bundle.RelationKeyName.String():                      pbtypes.String(r.GetName()),
			bundle.RelationKeyRelationDefaultValue.String():      pbtypes.NilToNullWrapper(r.GetDefaultValue()),
			bundle.RelationKeyRelationFormat.String():            pbtypes.Float64(float64(r.GetFormat())),
			bundle.RelationKeyRelationFormatObjectTypes.String(): pbtypes.StringList(r.GetObjectTypes()),
			bundle.RelationKeyRelationKey.String():               pbtypes.String(r.GetKey()),
			bundle.RelationKeyRelationMaxCount.String():          pbtypes.Float64(float64(r.GetMaxCount())),
			bundle.RelationKeyRelationReadonlyValue.String():     pbtypes.Bool(r.GetReadOnly()),
			bundle.RelationKeyScope.String():                     pbtypes.Float64(float64(r.GetScope())),
			bundle.RelationKeyType.String():                      pbtypes.String(bundle.TypeKeyRelation.BundledURL()),
			// TODO Is it ok?
			bundle.RelationKeyUniqueKey.String(): pbtypes.String(domain.RelationKey(r.GetKey()).URL()),
			bundle.RelationKeyRevision.String():  pbtypes.Int64(r.GetRevision()),
		},
	}
}

type Relations []*Relation

func (rs Relations) Models() []*model.Relation {
	res := make([]*model.Relation, 0, len(rs))
	for _, r := range rs {
		res = append(res, r.Relation)
	}
	return res
}

func (rs Relations) RelationLinks() []*model.RelationLink {
	res := make([]*model.RelationLink, 0, len(rs))
	for _, r := range rs {
		res = append(res, r.RelationLink())
	}
	return res
}

func (rs Relations) GetByKey(key string) *Relation {
	for _, r := range rs {
		if r.Key == key {
			return r
		}
	}
	return nil
}

func (rs Relations) GetModelByKey(key string) *model.Relation {
	if r := rs.GetByKey(key); r != nil {
		return r.Relation
	}
	return nil
}

func MigrateRelationModels(rels []*model.Relation) (relLinks []*model.RelationLink) {
	relLinks = make([]*model.RelationLink, 0, len(rels))
	for _, rel := range rels {
		relLinks = append(relLinks, &model.RelationLink{
			Key:    rel.Key,
			Format: rel.Format,
		})
	}
	return
}
