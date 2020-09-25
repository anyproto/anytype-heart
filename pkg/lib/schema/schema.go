package schema

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
)

var log = logging.Logger("anytype-core-schema")

type Schema struct {
	relations []*pbrelation.Relation
}

func New(relations []*pbrelation.Relation) Schema {
	return Schema{relations: relations}
}

func (sch *Schema) GetRelationByKey(key string) (*pbrelation.Relation, error) {
	for _, rel := range sch.relations {
		if rel.Key == key {
			return rel, nil
		}
	}
	return nil, fmt.Errorf("not found")
}

// Todo: data validation
