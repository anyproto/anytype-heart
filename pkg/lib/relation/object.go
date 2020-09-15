package relation

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
)

var ErrNotFound = fmt.Errorf("not found")

const bundledObjectTypeURLPrefix = "https://anytype.io/schemas/object/bundled/"

var (
	bundledObjectTypes = map[string]*relation.ObjectType{
		"page": {
			Name: "Page",
			Relations: []*relation.Relation{
				bundledRelations["creationDate"],
				bundledRelations["modifiedDate"],
				bundledRelations["name"],
				bundledRelations["iconEmoji"],
				bundledRelations["iconImage"],
				bundledRelations["coverImage"],
				bundledRelations["coverX"],
				bundledRelations["coverY"],
				bundledRelations["coverScale"],
			},
		},
	}
)

func GetObjectType(objectTypeURL string) (*relation.ObjectType, error) {
	if !strings.HasPrefix(objectTypeURL, bundledObjectTypeURLPrefix) {
		return nil, fmt.Errorf("invalid URL")
	}
	bundledId := strings.TrimPrefix(objectTypeURL, bundledObjectTypeURLPrefix)

	if v, exists := bundledObjectTypes[bundledId]; exists {
		return v, nil
	} else {
		return nil, ErrNotFound
	}
}
