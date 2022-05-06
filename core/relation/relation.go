package relation

import (
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

func RelationFromStruct(st *types.Struct) *Relation {
	return &Relation{
		Id: pbtypes.GetString(st, bundle.RelationKeyId.String()),
		Relation: &model.Relation{
			Key: pbtypes.GetString(st, bundle.RelationKeyRelationKey.String()),
			// TODO: get other fields
		},
	}
}

type Relation struct {
	Id string
	*model.Relation
}

func (r *Relation) RelationLink() *model.RelationLink {
	return &model.RelationLink{
		Id:  r.Id,
		Key: r.Key,
	}
}
