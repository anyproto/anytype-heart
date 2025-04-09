package userdataobject

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

var allowedDetailsToChange = []domain.RelationKey{bundle.RelationKeyDescription}

func AllowedDetailsToChange() []domain.RelationKey {
	return allowedDetailsToChange
}
