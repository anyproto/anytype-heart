package relationutils

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func OptionFromDetails(det *domain.Details) *Option {
	return &Option{
		RelationOption: &model.RelationOption{
			Id:          det.GetStringOrDefault(bundle.RelationKeyId, ""),
			Text:        det.GetStringOrDefault(bundle.RelationKeyName, ""),
			Color:       det.GetStringOrDefault(bundle.RelationKeyRelationOptionColor, ""),
			RelationKey: det.GetStringOrDefault(bundle.RelationKeyRelationKey, ""),
		},
	}
}

type Option struct {
	*model.RelationOption
}
