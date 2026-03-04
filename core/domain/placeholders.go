package domain

import (
	"fmt"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	PlaceholderToday       = "_today"
	PlaceholderCurrentUser = "_current_user"
)

type TemplatePlaceholder struct {
	RelationKey RelationKey
	Type        model.TemplatePlaceholderType
}

func PlaceholderTypeToString(t model.TemplatePlaceholderType) string {
	switch t {
	case model.TemplatePlaceholderType_TemplatePlaceholderToday:
		return PlaceholderToday
	case model.TemplatePlaceholderType_TemplatePlaceholderCurrentUser:
		return PlaceholderCurrentUser
	default:
		return ""
	}
}

func ParsePlaceholderType(s string) (model.TemplatePlaceholderType, error) {
	switch s {
	case PlaceholderToday:
		return model.TemplatePlaceholderType_TemplatePlaceholderToday, nil
	case PlaceholderCurrentUser:
		return model.TemplatePlaceholderType_TemplatePlaceholderCurrentUser, nil
	}
	return 0, fmt.Errorf("unknown placeholder type: %s", s)
}
