package relationutils

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
			Hidden:           det.GetBool(bundle.RelationKeyIsHidden),
			ReadOnly:         det.GetBool(bundle.RelationKeyRelationReadonlyValue),
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

func (r *Relation) ToDetails() *domain.Details {
	return domain.NewDetailsFromMap(map[domain.RelationKey]any{
		bundle.RelationKeyCreator:                   r.GetCreator(),
		bundle.RelationKeyDescription:               r.GetDescription(),
		bundle.RelationKeyId:                        r.Id,
		bundle.RelationKeyIsHidden:                  r.GetHidden(),
		bundle.RelationKeyIsReadonly:                r.GetReadOnlyRelation(),
		bundle.RelationKeyLayout:                    int64(model.ObjectType_relation),
		bundle.RelationKeyName:                      r.GetName(),
		bundle.RelationKeyRelationDefaultValue:      r.GetDefaultValue(),
		bundle.RelationKeyRelationFormat:            float64(r.GetFormat()),
		bundle.RelationKeyRelationFormatObjectTypes: r.GetObjectTypes(),
		bundle.RelationKeyRelationKey:               r.GetKey(),
		bundle.RelationKeyRelationMaxCount:          float64(r.GetMaxCount()),
		bundle.RelationKeyRelationReadonlyValue:     r.GetReadOnly(),
		bundle.RelationKeyScope:                     float64(r.GetScope()),
		bundle.RelationKeyType:                      bundle.TypeKeyRelation.BundledURL(),
		bundle.RelationKeyUniqueKey:                 domain.RelationKey(r.GetKey()).URL(),
		bundle.RelationKeyRevision:                  r.GetRevision(),
	})
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
