package schema

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var log = logging.Logger("anytype-core-schema")

type Schema struct {
	ObjType   *model.ObjectType
	Relations []*model.Relation
}

func New(objType *model.ObjectType, relations []*model.Relation) Schema {
	return Schema{ObjType: objType, Relations: relations}
}

func (sch *Schema) GetRelationByKey(key string) (*model.Relation, error) {
	if sch.Relations != nil {
		for _, rel := range sch.Relations {
			if rel.Key == key {
				return rel, nil
			}
		}
	}

	for _, rel := range sch.ObjType.Relations {
		if rel.Key == key {
			return rel, nil
		}
	}

	return nil, fmt.Errorf("not found")
}

// Todo: data validation
