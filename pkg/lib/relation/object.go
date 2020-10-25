package relation

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
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
