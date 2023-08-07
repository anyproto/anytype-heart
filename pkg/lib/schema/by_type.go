package schema

import (
	"fmt"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/samber/lo"
)

type schemaByType struct {
	ObjType           *model.ObjectType
	OptionalRelations []*model.RelationLink
}

func NewByType(objType *model.ObjectType, relations []*model.RelationLink) Schema {
	return &schemaByType{ObjType: objType, OptionalRelations: relations}
}

func (sch *schemaByType) ListRelations() []*model.RelationLink {
	allRelations := append(sch.ObjType.RelationLinks, sch.OptionalRelations...)
	return lo.UniqBy(allRelations, func(rel *model.RelationLink) string {
		return rel.Key
	})
}

func (sch *schemaByType) RequiredRelations() []*model.RelationLink {
	return []*model.RelationLink{bundle.MustGetRelationLink(bundle.RelationKeyName)}
}

func (sch *schemaByType) Filters() filter.Filter {
	relTypeFilter := filter.OrFilters{
		filter.Eq{
			Key:   bundle.RelationKeyType.String(),
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: pbtypes.String(sch.ObjType.Url),
		},
	}

	return relTypeFilter
}

func (sch *schemaByType) String() string {
	return fmt.Sprintf("type: %v", sch.ObjType.Url)
}
