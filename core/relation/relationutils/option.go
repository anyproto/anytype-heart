package relationutils

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

func OptionFromStruct(st *types.Struct) *Option {
	return &Option{
		RelationOption: &model.RelationOption{
			Id:          pbtypes.GetString(st, bundle.RelationKeyId.String()),
			Text:        pbtypes.GetString(st, bundle.RelationKeyName.String()),
			Color:       pbtypes.GetString(st, bundle.RelationKeyRelationOptionColor.String()),
			RelationKey: pbtypes.GetString(st, bundle.RelationKeyRelationKey.String()),
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
			bundle.RelationKeyType.String():                pbtypes.String(bundle.TypeKeyRelationOption.URL()),
			bundle.RelationKeyName.String():                pbtypes.String(o.Text),
			bundle.RelationKeyRelationOptionColor.String(): pbtypes.String(o.Color),
			bundle.RelationKeyRelationKey.String():         pbtypes.String(o.RelationKey),
		},
	}
}
