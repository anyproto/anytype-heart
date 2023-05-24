package relationutils

import (
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
			bundle.RelationKeyLayout.String():              pbtypes.Int64(int64(model.ObjectType_relationOption)),
		},
	}
}
