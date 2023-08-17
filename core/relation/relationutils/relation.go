package relationutils

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func RelationFromStruct(st *types.Struct) *Relation {
	key := pbtypes.GetString(st, bundle.RelationKeyRelationKey.String())
	maxCount := int32(pbtypes.GetFloat64(st, bundle.RelationKeyRelationMaxCount.String()))
	return &Relation{
		Relation: &model.Relation{
			Id:               pbtypes.GetString(st, bundle.RelationKeyId.String()),
			Key:              key,
			Format:           model.RelationFormat(pbtypes.GetFloat64(st, bundle.RelationKeyRelationFormat.String())),
			Name:             pbtypes.GetString(st, bundle.RelationKeyName.String()),
			DefaultValue:     pbtypes.Get(st, bundle.RelationKeyRelationDefaultValue.String()),
			DataSource:       model.Relation_details,
			Hidden:           pbtypes.GetBool(st, bundle.RelationKeyIsHidden.String()),
			ReadOnly:         pbtypes.GetBool(st, bundle.RelationKeyIsReadonly.String()),
			ReadOnlyRelation: false,
			Multi:            maxCount > 1,
			ObjectTypes:      pbtypes.GetStringList(st, bundle.RelationKeyRelationFormatObjectTypes.String()),
			MaxCount:         maxCount,
			Description:      pbtypes.GetString(st, bundle.RelationKeyDescription.String()),
			Scope:            model.RelationScope(pbtypes.GetFloat64(st, bundle.RelationKeyScope.String())),
			Creator:          pbtypes.GetString(st, bundle.RelationKeyCreator.String()),
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
			bundle.RelationKeyUniqueKey.String(): pbtypes.String(bundle.RelationKey(r.GetKey()).URL()),
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
