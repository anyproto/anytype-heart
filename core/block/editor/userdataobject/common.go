package userdataobject

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

var AllowedDetailsToChange = []domain.RelationKey{bundle.RelationKeyDescription}
