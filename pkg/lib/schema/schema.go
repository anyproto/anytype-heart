package schema

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"strings"
)

var log = logging.Logger("anytype-core-schema")

// Schema used to subset compatible objects by some common relations
type Schema interface {
	Filters() filter.Filter
	ObjectType() *model.ObjectType
	String() string      // describes the schema
	Description() string // describes the schema

	ListRelations() []*model.Relation
	RequiredRelations() []*model.Relation
}

type schemaByType struct {
	ObjType           *model.ObjectType
	OptionalRelations []*model.Relation
}

type schemaByRelations struct {
	ObjectTypeIds     []string
	CommonRelations   []*model.Relation
	OptionalRelations []*model.Relation
}

func NewByType(objType *model.ObjectType, relations []*model.Relation) Schema {
	return &schemaByType{ObjType: objType, OptionalRelations: relations}
}

func NewByRelations(objTypes []string, commonRelations, optionalRelations []*model.Relation) Schema {
	return &schemaByRelations{ObjectTypeIds: objTypes, OptionalRelations: optionalRelations, CommonRelations: commonRelations}
}

func (sch *schemaByType) ListRelations() []*model.Relation {
	var total = len(sch.OptionalRelations) + len(sch.ObjType.Relations)

	var m = make(map[string]struct{}, total)
	var rels = make([]*model.Relation, 0, total)
	for _, rel := range append(sch.ObjType.Relations, sch.OptionalRelations...) {
		if _, exists := m[rel.Key]; exists {
			continue
		}
		m[rel.Key] = struct{}{}
		rels = append(rels, rel)
	}

	return rels
}

func (sch *schemaByType) RequiredRelations() []*model.Relation {
	return []*model.Relation{bundle.MustGetRelation(bundle.RelationKeyName)}
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

func (sch *schemaByRelations) RequiredRelations() []*model.Relation {
	rels := sch.CommonRelations
	if !pbtypes.HasRelation(rels, bundle.RelationKeyName.String()) {
		rels = append([]*model.Relation{bundle.MustGetRelation(bundle.RelationKeyName)}, rels...)
	}
	return rels
}

func (sch *schemaByRelations) ListRelations() []*model.Relation {
	var total = len(sch.OptionalRelations) + len(sch.CommonRelations)

	var m = make(map[string]struct{}, total)
	var rels = make([]*model.Relation, 0, total)
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
	relTypeFilter := filter.OrFilters{
		filter.In{
			Key:   bundle.RelationKeyType.String(),
			Value: pbtypes.StringList(sch.ObjectTypeIds).GetListValue(),
		},
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
	return fmt.Sprintf("relations: %v", pbtypes.GetRelationKeys(sch.CommonRelations))
}

func (sch *schemaByRelations) Description() string {
	var relName []string
	for _, rel := range sch.CommonRelations {
		relName = append(relName, rel.Name)
	}
	if len(relName) == 1 {
		return fmt.Sprintf("Relation %s", relName[0])
	}

	return fmt.Sprintf("Relations %s", strings.Join(relName, ", "))
}
