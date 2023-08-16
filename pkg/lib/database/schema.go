package database

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Schema used to subset compatible objects by some common relations
type Schema interface {
	ListRelations() []*model.RelationLink
	RequiredRelations() []*model.RelationLink
}
