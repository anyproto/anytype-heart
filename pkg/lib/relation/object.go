package relation

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	st "github.com/gogo/protobuf/types"
	"github.com/google/martian/log"
	"go4.org/sort"
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
		"profile": {
			Url:  bundledObjectTypeURLPrefix + "profile",
			Name: "Profile",
			Relations: []*relation.Relation{
				MustGetBundledRelationByKey("id"),
				MustGetBundledRelationByKey("type"),
				MustGetBundledRelationByKey("name"),
				MustGetBundledRelationByKey("createdDate"),
				MustGetBundledRelationByKey("lastModifiedDate"),
				MustGetBundledRelationByKey("lastOpenedDate"),

				MustGetBundledRelationByKey("iconImage"),
				MustGetBundledRelationByKey("coverImage"),
				MustGetBundledRelationByKey("coverX"),
				MustGetBundledRelationByKey("coverY"),
				MustGetBundledRelationByKey("coverScale"),
			},
			Layout:    relation.ObjectType_contact,
			IconEmoji: "👤",
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
		"objectType": {
			Url:  bundledObjectTypeURLPrefix + "objectType",
			Name: "Object Type",
			Relations: []*relation.Relation{
				MustGetBundledRelationByKey("id"),
				MustGetBundledRelationByKey("name"),
				MustGetBundledRelationByKey("createdDate"),
				MustGetBundledRelationByKey("lastModifiedDate"),
				MustGetBundledRelationByKey("lastOpenedDate"),
				MustGetBundledRelationByKey("iconEmoji"),
				MustGetBundledRelationByKey("iconImage"),
			},
			Layout:    relation.ObjectType_objectType,
			IconEmoji: "ℹ️",
		},
	}
)

func GetObjectType(objectTypeURL string) (*relation.ObjectType, error) {
	if !strings.HasPrefix(objectTypeURL, bundledObjectTypeURLPrefix) {
		return nil, fmt.Errorf("invalid URL")
	}
	bundledId := strings.TrimPrefix(objectTypeURL, bundledObjectTypeURLPrefix)

	if v, exists := BundledObjectTypes[bundledId]; exists {
		return pbtypes.CopyObjectType(v), nil
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

func MergeAndSortRelations(objTypeRelations []*relation.Relation, extraRelations []*relation.Relation, details *st.Struct) []*relation.Relation {
	var m = make(map[string]struct{}, len(extraRelations))
	var rels = make([]*relation.Relation, 0, len(objTypeRelations)+len(extraRelations))

	for _, rel := range extraRelations {
		m[rel.Key] = struct{}{}
		rels = append(rels, pbtypes.CopyRelation(rel))
	}

	for _, rel := range objTypeRelations {
		if _, exists := m[rel.Key]; exists {
			continue
		}
		rels = append(rels, pbtypes.CopyRelation(rel))
		m[rel.Key] = struct{}{}
	}

	if details == nil || details.Fields == nil {
		return rels
	}

	sort.Slice(rels, func(i, j int) bool {
		_, iExists := details.Fields[rels[i].Key]
		_, jExists := details.Fields[rels[j].Key]

		if iExists && !jExists {
			return true
		}

		return false
	})

	return rels
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
