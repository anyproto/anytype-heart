package schema

import (
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Schema used to subset compatible objects by some common relations
type Schema interface {
	Filters() filter.Filter
	ListRelations() []*model.RelationLink
	RequiredRelations() []*model.RelationLink
}
