package bundle

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

// all required internal relations will be added to any new object type
var RequiredInternalRelations = []string{"id", "name", "type", "createdDate", "lastModifiedDate", "lastModifiedBy", "lastOpenedDate"}
var FormatFilePossibleTargetObjectTypes = []string{objects.BundledObjectTypeURLPrefix + "file", objects.BundledObjectTypeURLPrefix + "image", objects.BundledObjectTypeURLPrefix + "video", objects.BundledObjectTypeURLPrefix + "audio"}

// filled in init
var LocalOnlyRelationsKeys []string
var ErrNotFound = fmt.Errorf("not found")

func init() {
	for _, r := range Relations {
		if r.DataSource == relation.Relation_derived {
			LocalOnlyRelationsKeys = append(LocalOnlyRelationsKeys, r.Key)
		}
	}
}

func GetType(u string) (*relation.ObjectType, error) {
	if !strings.HasPrefix(u, TypePrefix) {
		return nil, fmt.Errorf("invalid url with no bundled type prefix")
	}
	tk := TypeKey(strings.TrimPrefix(u, TypePrefix))
	if v, exists := Types[tk]; exists {
		return pbtypes.CopyObjectType(v), nil
	}

	return nil, ErrNotFound
}

func ListTypes() ([]*relation.ObjectType, error) {
	var otypes []*relation.ObjectType
	for _, ot := range Types {
		otypes = append(otypes, ot)
	}

	return otypes, nil
}
