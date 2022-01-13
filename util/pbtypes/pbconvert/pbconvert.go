package pbconvert

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

func RelationToValue(rel *model.Relation) *types.Value {
	return &types.Value{Kind: &types.Value_StructValue{
		StructValue: &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String():             pbtypes.String(rel.Key),
				bundle.RelationKeyRelationFormat.String(): pbtypes.Float64(float64(rel.Format)),
				bundle.RelationKeyName.String():           pbtypes.String(rel.Name),
				"defaultValue":                            rel.DefaultValue,
				"dataSource":                              pbtypes.Int64(int64(rel.DataSource)),
				bundle.RelationKeyIsHidden.String():       pbtypes.Bool(rel.Hidden),
				"readOnly":                                pbtypes.Bool(rel.ReadOnly),
				bundle.RelationKeyIsReadonly.String():     pbtypes.Bool(rel.ReadOnlyRelation),
				"multi":                                   pbtypes.Bool(rel.Multi),
				"objectTypes":                             pbtypes.StringList(rel.ObjectTypes),
				"maxCount":                                pbtypes.Int64(int64(rel.MaxCount)),
				bundle.RelationKeyDescription.String():    pbtypes.String(rel.Description),
				"scope":                                   pbtypes.Int64(int64(rel.Scope)),
				bundle.RelationKeyCreator.String():        pbtypes.String(rel.Creator),
				bundle.RelationKeyType.String():           pbtypes.String(bundle.TypeKeyRelation.URL()),
				bundle.RelationKeyLayout.String():         pbtypes.Float64(float64(model.ObjectType_relation)),
			},
		},
	}}
}

func StructToRelation(s *types.Struct) *model.Relation {
	if s == nil {
		return nil
	}
	return &model.Relation{
		Key:              pbtypes.GetString(s, bundle.RelationKeyId.String()),
		Format:           model.RelationFormat(pbtypes.GetInt64(s, bundle.RelationKeyRelationFormat.String())),
		Name:             pbtypes.GetString(s, bundle.RelationKeyName.String()),
		DefaultValue:     pbtypes.Get(s, "defaultValue"),
		DataSource:       model.RelationDataSource(pbtypes.GetInt64(s, "dataSource")),
		Hidden:           pbtypes.GetBool(s, bundle.RelationKeyIsHidden.String()),
		ReadOnly:         pbtypes.GetBool(s, "readOnly"),
		ReadOnlyRelation: pbtypes.GetBool(s, bundle.RelationKeyIsReadonly.String()),
		Multi:            pbtypes.GetBool(s, "multi"),
		ObjectTypes:      pbtypes.GetStringList(s, "objectTypes"),
		SelectDict:       nil,
		MaxCount:         int32(pbtypes.GetInt64(s, "maxCount")),
		Description:      pbtypes.GetString(s, bundle.RelationKeyDescription.String()),
		Scope:            model.RelationScope(pbtypes.GetInt64(s, "scope")),
		Creator:          pbtypes.GetString(s, bundle.RelationKeyCreator.String()),
	}
}

func RelationOptionToValue(opt *model.RelationOption) *types.Value {
	return &types.Value{Kind: &types.Value_StructValue{
		StructValue: &types.Struct{
			Fields: map[string]*types.Value{
				"id":    pbtypes.String(opt.Id),
				"text":  pbtypes.String(opt.Text),
				"color": pbtypes.String(opt.Color),
				"scope": pbtypes.Int64(int64(opt.Scope)),
			},
		}}}
}

func StructToRelationOption(s *types.Struct) *model.RelationOption {
	if s == nil {
		return nil
	}
	return &model.RelationOption{
		Id:    pbtypes.GetString(s, "id"),
		Text:  pbtypes.GetString(s, "text"),
		Color: pbtypes.GetString(s, "color"),
		Scope: model.RelationOptionScope(pbtypes.GetInt64(s, "scope")),
	}
}
