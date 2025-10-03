package relationutils

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func OptionFromDetails(det *domain.Details) *Option {
	return &Option{
		RelationOption: &model.RelationOption{
			Id:          det.GetString(bundle.RelationKeyId),
			Text:        det.GetString(bundle.RelationKeyName),
			Color:       det.GetString(bundle.RelationKeyRelationOptionColor),
			RelationKey: det.GetString(bundle.RelationKeyRelationKey),
			OrderId:     det.GetString(bundle.RelationKeyOrderId),
		},
	}
}

type Option struct {
	*model.RelationOption
}
