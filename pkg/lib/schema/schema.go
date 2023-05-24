package schema

import (
	"fmt"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"strings"
)

// Schema used to subset compatible objects by some common relations
type Schema interface {
	Filters() filter.Filter
	ObjectType() *model.ObjectType
	String() string      // describes the schema
	Description() string // describes the schema

	ListRelations() []*model.RelationLink
	RequiredRelations() []*model.RelationLink
}

type schemaByType struct {
	ObjType           *model.ObjectType
	OptionalRelations []*model.RelationLink
}

type schemaByRelations struct {
	ObjectTypeIds     []string
	CommonRelations   []*model.RelationLink
	OptionalRelations []*model.RelationLink
}

func NewByType(objType *model.ObjectType, relations []*model.RelationLink) Schema {
	return &schemaByType{ObjType: objType, OptionalRelations: relations}
}

func NewByRelations(objTypes []string, commonRelations, optionalRelations []*model.RelationLink) Schema {
	return &schemaByRelations{ObjectTypeIds: objTypes, OptionalRelations: optionalRelations, CommonRelations: commonRelations}
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

func (sch *schemaByType) ObjectType() *model.ObjectType {
	return sch.ObjType
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

func (sch *schemaByType) Description() string {
	return fmt.Sprintf("%s", sch.ObjType.Name)
}

func (sch *schemaByRelations) RequiredRelations() []*model.RelationLink {
	var rels = []*model.RelationLink{
		bundle.MustGetRelationLink(bundle.RelationKeyName), bundle.MustGetRelationLink(bundle.RelationKeyType)}
	for _, rel := range sch.CommonRelations {
		if !pbtypes.HasRelationLink(rels, rel.Key) {
			rels = append(rels, rel)
		}
	}
	return rels
}

func (sch *schemaByRelations) ListRelations() []*model.RelationLink {
	var total = len(sch.OptionalRelations) + len(sch.CommonRelations)

	var m = make(map[string]struct{}, total)
	var rels = make([]*model.RelationLink, 0, total)
	for _, rel := range append(sch.CommonRelations, sch.OptionalRelations...) {
		if _, exists := m[rel.Key]; exists {
			continue
		}
		m[rel.Key] = struct{}{}
		rels = append(rels, rel)
	}

	return rels
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
func (sch *schemaByRelations) ObjectType() *model.ObjectType {
	return nil
}

func (sch *schemaByRelations) String() string {
	return fmt.Sprintf("relations: %v", pbtypes.GetRelationListKeys(sch.CommonRelations))
}

func (sch *schemaByRelations) Description() string {
	var relName []string
	for _, rel := range sch.CommonRelations {
		relName = append(relName, rel.Key)
	}
	if len(relName) == 1 {
		return fmt.Sprintf("Relation %s", relName[0])
	}

	return fmt.Sprintf("Relations %s", strings.Join(relName, ", "))
}
