package domain

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	PlaceholderToday       = "_today"
	PlaceholderCurrentUser = "_current_user"
)

func IsPlaceholder(v Value) bool {
	s, ok := v.TryString()
	if !ok {
		return false
	}
	switch s {
	case PlaceholderToday, PlaceholderCurrentUser:
		return true
	}
	return false
}

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
