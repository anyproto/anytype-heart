package relationutils

import (
	"strings"

	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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
			bundle.RelationKeyId.String():                        pbtypes.String(r.Id),
			bundle.RelationKeyRelationKey.String():               pbtypes.String(r.GetKey()),
			bundle.RelationKeyRelationFormat.String():            pbtypes.Float64(float64(r.GetFormat())),
			bundle.RelationKeyName.String():                      pbtypes.String(r.GetName()),
			bundle.RelationKeyType.String():                      pbtypes.String(bundle.TypeKeyRelation.URL()),
			bundle.RelationKeyLayout.String():                    pbtypes.Int64(int64(model.ObjectType_relation)),
			bundle.RelationKeyRelationDefaultValue.String():      pbtypes.NilToNullWrapper(r.GetDefaultValue()),
			bundle.RelationKeyIsHidden.String():                  pbtypes.Bool(r.GetHidden()),
			bundle.RelationKeyRelationReadonlyValue.String():     pbtypes.Bool(r.GetReadOnly()),
			bundle.RelationKeyRelationFormatObjectTypes.String(): pbtypes.StringList(r.GetObjectTypes()),
			bundle.RelationKeyRelationMaxCount.String():          pbtypes.Float64(float64(r.GetMaxCount())),
			bundle.RelationKeyDescription.String():               pbtypes.String(r.GetDescription()),
			bundle.RelationKeyScope.String():                     pbtypes.Float64(float64(r.GetScope())),
			bundle.RelationKeyCreator.String():                   pbtypes.String(r.GetCreator()),
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

func MigrateRelationIds(ids []string) []string {
	// shortcut if there is nothing to migrate
	hasIdsToMigrate := false
	for _, id := range ids {
		if strings.HasPrefix(id, addr.BundledRelationURLPrefix) || strings.HasPrefix(id, addr.OldIndexedRelationURLPrefix) {
			hasIdsToMigrate = true
			break
		}
	}
	if !hasIdsToMigrate {
		return ids
	}

	normalized := make([]string, len(ids))
	var (
		key string
		err error
	)
	for i, id := range ids {
		key, err = pbtypes.RelationIdToKey(id)
		if err != nil {
			normalized[i] = id
		} else {
			normalized[i] = addr.RelationKeyToIdPrefix + key
		}
	}
	return normalized
}
