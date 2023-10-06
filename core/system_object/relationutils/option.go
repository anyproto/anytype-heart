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
