package schema

import (
	"fmt"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/samber/lo"
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
	alwaysRequired := []*model.RelationLink{
		bundle.MustGetRelationLink(bundle.RelationKeyName),
		bundle.MustGetRelationLink(bundle.RelationKeyType),
	}
	required := append(alwaysRequired, sch.CommonRelations...)
	return lo.UniqBy(required, func(rel *model.RelationLink) string {
		return rel.Key
	})
}

func (sch *schemaByRelations) ListRelations() []*model.RelationLink {
	allRelations := append(sch.CommonRelations, sch.OptionalRelations...)
	return lo.UniqBy(allRelations, func(rel *model.RelationLink) string {
		return rel.Key
	})
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
