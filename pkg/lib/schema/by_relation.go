package schema

import (
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"fmt"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type schemaByRelations struct {
	ObjectTypeIds     []string
	CommonRelations   []*model.RelationLink
	OptionalRelations []*model.RelationLink
}

func NewByRelations(objTypes []string, commonRelations, optionalRelations []*model.RelationLink) Schema {
	return &schemaByRelations{ObjectTypeIds: objTypes, OptionalRelations: optionalRelations, CommonRelations: commonRelations}
}

func (sch *schemaByRelations) RequiredRelations() []*model.RelationLink {
	required := []*model.RelationLink{
		bundle.MustGetRelationLink(bundle.RelationKeyName),
		bundle.MustGetRelationLink(bundle.RelationKeyType),
	}
	for _, rel := range sch.CommonRelations {
		if !pbtypes.HasRelationLink(required, rel.Key) {
			required = append(required, rel)
		}
	}
	return required
}

func (sch *schemaByRelations) ListRelations() []*model.RelationLink {
	total := len(sch.OptionalRelations) + len(sch.CommonRelations)
	uniq := make(map[string]struct{}, total)
	relations := make([]*model.RelationLink, 0, total)
	for _, rel := range append(sch.CommonRelations, sch.OptionalRelations...) {
		if _, exists := uniq[rel.Key]; exists {
			continue
		}
		uniq[rel.Key] = struct{}{}
		relations = append(relations, rel)
	}

	return relations
}

func (sch *schemaByRelations) Filters() filter.Filter {
	var relTypeFilter filter.OrFilters

	if len(sch.ObjectTypeIds) > 0 {
		relTypeFilter = append(relTypeFilter, filter.In{
			Key:   bundle.RelationKeyType.String(),
			Value: pbtypes.StringList(sch.ObjectTypeIds).GetListValue(),
		})
	}

	for _, rel := range sch.CommonRelations {
		relTypeFilter = append(relTypeFilter, filter.Exists{
			Key: rel.Key,
		})
	}

	return relTypeFilter
}

func (sch *schemaByRelations) String() string {
	return fmt.Sprintf("relations: %v", pbtypes.GetRelationListKeys(sch.CommonRelations))
}
