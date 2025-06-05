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
			Creator:          det.GetString(bundle.RelationKeyCreator),
			Revision:         det.GetInt64(bundle.RelationKeyRevision),
			IncludeTime:      det.GetBool(bundle.RelationKeyRelationFormatIncludeTime),
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
	return domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
		bundle.RelationKeyCreator:                   domain.String(r.GetCreator()),
		bundle.RelationKeyDescription:               domain.String(r.GetDescription()),
		bundle.RelationKeyId:                        domain.String(r.Id),
		bundle.RelationKeyIsHidden:                  domain.Bool(r.GetHidden()),
		bundle.RelationKeyIsReadonly:                domain.Bool(r.GetReadOnlyRelation()),
		bundle.RelationKeyResolvedLayout:            domain.Int64(int64(model.ObjectType_relation)),
		bundle.RelationKeyLayout:                    domain.Int64(int64(model.ObjectType_relation)),
		bundle.RelationKeyName:                      domain.String(r.GetName()),
		bundle.RelationKeyRelationDefaultValue:      domain.ValueFromProto(r.GetDefaultValue()),
		bundle.RelationKeyRelationFormat:            domain.Float64(float64(r.GetFormat())),
		bundle.RelationKeyRelationFormatObjectTypes: domain.StringList(r.GetObjectTypes()),
		bundle.RelationKeyRelationKey:               domain.String(r.GetKey()),
		bundle.RelationKeyRelationMaxCount:          domain.Float64(float64(r.GetMaxCount())),
		bundle.RelationKeyRelationReadonlyValue:     domain.Bool(r.GetReadOnly()),
		bundle.RelationKeyType:                      domain.String(bundle.TypeKeyRelation.BundledURL()),
		// TODO Is it ok?
		bundle.RelationKeyUniqueKey:                 domain.String(domain.RelationKey(r.GetKey()).URL()),
		bundle.RelationKeyRevision:                  domain.Int64(r.GetRevision()),
		bundle.RelationKeyRelationFormatIncludeTime: domain.Bool(r.GetIncludeTime()),
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
