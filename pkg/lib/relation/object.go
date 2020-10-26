package relation

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	st "github.com/gogo/protobuf/types"
	"github.com/google/martian/log"
)

var ErrNotFound = fmt.Errorf("not found")

const bundledObjectTypeURLPrefix = "https://anytype.io/schemas/object/bundled/"

var (
	BundledObjectTypes = map[string]*relation.ObjectType{
		"page": {
			Url:  bundledObjectTypeURLPrefix + "page",
			Name: "Page",
			Relations: []*relation.Relation{
				MustGetBundledRelationByKey("id"),
				MustGetBundledRelationByKey("type"),
				MustGetBundledRelationByKey("name"),
				MustGetBundledRelationByKey("createdDate"),
				MustGetBundledRelationByKey("lastModifiedDate"),
				MustGetBundledRelationByKey("lastOpenedDate"),

				MustGetBundledRelationByKey("iconEmoji"),
				MustGetBundledRelationByKey("iconImage"),
				MustGetBundledRelationByKey("coverImage"),
				MustGetBundledRelationByKey("coverX"),
				MustGetBundledRelationByKey("coverY"),
				MustGetBundledRelationByKey("coverScale"),
			},
			Layout:    relation.ObjectType_page,
			IconEmoji: "📒",
		},
		"set": {
			Url:  bundledObjectTypeURLPrefix + "set",
			Name: "Set of objects",
			Relations: []*relation.Relation{
				MustGetBundledRelationByKey("id"),
				MustGetBundledRelationByKey("type"),
				MustGetBundledRelationByKey("name"),
				MustGetBundledRelationByKey("createdDate"),
				MustGetBundledRelationByKey("lastModifiedDate"),
				MustGetBundledRelationByKey("lastOpenedDate"),
				MustGetBundledRelationByKey("iconEmoji"),
				MustGetBundledRelationByKey("iconImage"),
				MustGetBundledRelationByKey("coverImage"),
				MustGetBundledRelationByKey("coverX"),
				MustGetBundledRelationByKey("coverY"),
				MustGetBundledRelationByKey("coverScale"),
			},
			Layout:    relation.ObjectType_set,
			IconEmoji: "🗂",
		},
	}
)

func GetObjectType(objectTypeURL string) (*relation.ObjectType, error) {
	if !strings.HasPrefix(objectTypeURL, bundledObjectTypeURLPrefix) {
		return nil, fmt.Errorf("invalid URL")
	}
	bundledId := strings.TrimPrefix(objectTypeURL, bundledObjectTypeURLPrefix)

	if v, exists := BundledObjectTypes[bundledId]; exists {
		return v, nil
	} else {
		return nil, ErrNotFound
	}
}

func ListObjectTypes() ([]*relation.ObjectType, error) {
	var otypes []*relation.ObjectType
	for _, ot := range BundledObjectTypes {
		otypes = append(otypes, ot)
	}

	return otypes, nil
}

func MergeRelations(relations []*relation.Relation) []*relation.Relation {
	var m = map[string]*relation.Relation{}
	for _, rel := range relations {
		m[rel.Key] = pbtypes.CopyRelation(rel)
	}

	var rels = make([]*relation.Relation, 0, len(m))
	for i := range m {
		rels = append(rels, m[i])
	}

	return rels
}

func FillRelations(relations []*relation.Relation, details *st.Struct) []*relation.RelationWithValue {
	if details == nil || details.Fields == nil {
		return nil
	}

	var m = map[string]*relation.Relation{}
	for _, rel := range relations {
		m[rel.Key] = pbtypes.CopyRelation(rel)
	}

	var rels = make([]*relation.RelationWithValue, 0, len(details.Fields))
	for key, val := range details.Fields {
		if v, exists := m[key]; !exists {
			log.Errorf("FillRelations: detail has key that doesn't exists in the relations: %s", key)
			continue
		} else {
			rels = append(rels, &relation.RelationWithValue{Relation: v, Value: val})
		}
	}

	return rels
}
