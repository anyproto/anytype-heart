package schema

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"fmt"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
)

type schemaByType struct {
	ObjType           *model.ObjectType
	OptionalRelations []*model.RelationLink
}

func NewByType(objType *model.ObjectType, relations []*model.RelationLink) Schema {
	return &schemaByType{ObjType: objType, OptionalRelations: relations}
}

func (sch *schemaByType) ListRelations() []*model.RelationLink {
	var total = len(sch.OptionalRelations) + len(sch.ObjType.RelationLinks)

	var m = make(map[string]struct{}, total)
	var rels = make([]*model.RelationLink, 0, total)
	for _, rel := range append(sch.ObjType.RelationLinks, sch.OptionalRelations...) {
		if _, exists := m[rel.Key]; exists {
			continue
		}
		m[rel.Key] = struct{}{}
		rels = append(rels, rel)
	}

	return rels
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
