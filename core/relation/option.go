package relation

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

func OptionFromStruct(st *types.Struct) *Option {
	return &Option{
		RelationOption: &model.RelationOption{
			Id:    pbtypes.GetString(st, bundle.RelationKeyId.String()),
			Text:  pbtypes.GetString(st, bundle.RelationKeyRelationOptionText.String()),
			Color: pbtypes.GetString(st, bundle.RelationKeyRelationOptionColor.String()),
			Scope: model.RelationOptionScope(pbtypes.GetInt64(st, bundle.RelationKeyScope.String())),
		},
	}
}

type Option struct {
	*model.RelationOption
}

func (o *Option) ToStruct() *types.Struct {
	return &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():                  pbtypes.String(o.Id),
			bundle.RelationKeyScope.String():               pbtypes.Int64(int64(o.Scope)),
			bundle.RelationKeyRelationOptionText.String():  pbtypes.String(o.Text),
			bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(o.Color),
		},
	}
}
