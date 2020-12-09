package database

import (
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/gogo/protobuf/types"
)

type Ctrl interface {
	Anytype() anytype.Service

	SetDetails(ctx *state.Context, req pb.RpcBlockSetDetailsRequest) error
	GetRelations(objectId string) (relations []*pbrelation.Relation, err error)

	CreateSmartBlock(sbType coresb.SmartBlockType, details *types.Struct, objectTypes []string, relations []*pbrelation.Relation) (id string, err error)
	GetObjectType(url string) (objectType *pbrelation.ObjectType, err error)
	UpdateExtraRelations(id string, relations []*pbrelation.Relation, createIfMissing bool) (err error)
	AddExtraRelations(id string, relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error)
	RemoveExtraRelations(id string, relationKeys []string) (err error)

	AddObjectTypes(objectId string, objectTypes []string) (err error)
	RemoveObjectTypes(objectId string, objectTypes []string) (err error)
}

type Router interface {
	Get(id string) (database.Database, error)
}

func New(s Ctrl) Router {
	return &router{s: s}
}

type router struct{ s Ctrl }

func (r router) Get(id string) (database.Database, error) {
	setDetailsNoContext := func(req pb.RpcBlockSetDetailsRequest) error {
		return r.s.SetDetails(nil, req)
	}

	setOrAddRelations := func(id string, relations []*pbrelation.Relation) error {
		return r.s.UpdateExtraRelations(id, relations, true)
	}

	return objects.New(
		r.s.Anytype().ObjectStore(),
		id,
		setDetailsNoContext,
		r.s.GetRelations,
		setOrAddRelations,
		r.s.CreateSmartBlock,
	), nil
}
