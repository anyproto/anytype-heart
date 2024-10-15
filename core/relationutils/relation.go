package relationutils

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func RelationFromDetails(det *domain.Details) *Relation {
	key := det.GetString(bundle.RelationKeyRelationKey)
	maxCount := int32(det.GetInt64(bundle.RelationKeyRelationMaxCount))
	return &Relation{
		Relation: &model.Relation{
			Id:               det.GetString(bundle.RelationKeyId),
			Key:              key,
			Format:           model.RelationFormat(det.GetFloat64(bundle.RelationKeyRelationFormat)),
			Name:             det.GetString(bundle.RelationKeyName),
			DataSource:       model.Relation_details,
			Hidden:           det.GetBool(bundle.RelationKeyIsHidden),
			ReadOnly:         det.GetBool(bundle.RelationKeyRelationReadonlyValue),
			ReadOnlyRelation: false,
			Multi:            maxCount > 1,
			ObjectTypes:      det.GetStringList(bundle.RelationKeyRelationFormatObjectTypes),
			MaxCount:         maxCount,
			Description:      det.GetString(bundle.RelationKeyDescription),
			Scope:            model.RelationScope(det.GetFloat64(bundle.RelationKeyScope)),
			Creator:          det.GetString(bundle.RelationKeyCreator),
			Revision:         int64(det.GetInt64(bundle.RelationKeyRevision)),
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
	det := domain.NewDetails()
	det.SetString(bundle.RelationKeyCreator, r.GetCreator())
	det.SetString(bundle.RelationKeyDescription, r.GetDescription())
	det.SetString(bundle.RelationKeyId, r.Id)
	det.SetBool(bundle.RelationKeyIsHidden, r.GetHidden())
	det.SetBool(bundle.RelationKeyIsReadonly, r.GetReadOnlyRelation())
	det.SetInt64(bundle.RelationKeyLayout, int64(model.ObjectType_relation))
	det.SetString(bundle.RelationKeyName, r.GetName())
	det.SetProtoValue(bundle.RelationKeyRelationDefaultValue, r.GetDefaultValue())
	det.SetInt64(bundle.RelationKeyRelationFormat, int64(r.GetFormat()))
	det.SetStringList(bundle.RelationKeyRelationFormatObjectTypes, r.GetObjectTypes())
	det.SetString(bundle.RelationKeyRelationKey, r.GetKey())
	det.SetInt64(bundle.RelationKeyRelationMaxCount, int64(r.GetMaxCount()))
	det.SetBool(bundle.RelationKeyRelationReadonlyValue, r.GetReadOnly())
	det.SetInt64(bundle.RelationKeyScope, int64(r.GetScope()))
	det.SetString(bundle.RelationKeyType, bundle.TypeKeyRelation.BundledURL())
	det.SetString(bundle.RelationKeyUniqueKey, domain.RelationKey(r.GetKey()).URL())
	det.SetInt64(bundle.RelationKeyRevision, r.GetRevision())
	return det
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
