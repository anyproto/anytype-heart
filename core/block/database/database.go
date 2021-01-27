package database

import (
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

type Ctrl interface {
	Anytype() anytype.Service

	SetDetails(ctx *state.Context, req pb.RpcBlockSetDetailsRequest) error
	GetRelations(objectId string) (relations []*pbrelation.Relation, err error)

	CreateSmartBlock(sbType coresb.SmartBlockType, details *types.Struct, relations []*pbrelation.Relation) (id string, newDetails *types.Struct, err error)
	GetObjectType(url string) (objectType *pbrelation.ObjectType, err error)
	UpdateExtraRelations(id string, relations []*pbrelation.Relation, createIfMissing bool) (err error)
	AddExtraRelations(id string, relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error)
	RemoveExtraRelations(id string, relationKeys []string) (err error)
	ModifyExtraRelations(objectId string, modifier func(current []*pbrelation.Relation) ([]*pbrelation.Relation, error)) (err error)
	AddExtraRelationOption(ctx *state.Context, req pb.RpcObjectRelationOptionAddRequest) (opt *pbrelation.RelationOption, err error)

	SetObjectTypes(objectId string, objectTypes []string) (err error)
}

type Router interface {
	Get(id string) (database.Database, error)
}

func New(s Ctrl) Router {
	return &router{s: s}
}

type router struct{ s Ctrl }

func (r router) Get(id string) (database.Database, error) {
	// todo: wrap into iface
	setDetailsNoContext := func(req pb.RpcBlockSetDetailsRequest) error {
		return r.s.SetDetails(nil, req)
	}

	addOptionNoContext := func(req pb.RpcObjectRelationOptionAddRequest) (opt *pbrelation.RelationOption, err error) {
		return r.s.AddExtraRelationOption(nil, req)
	}

	setOrAddRelations := func(id string, relations []*pbrelation.Relation) error {
		newRels := pbtypes.CopyRelations(relations)
		return r.s.ModifyExtraRelations(id, func(current []*pbrelation.Relation) ([]*pbrelation.Relation, error) {
			for _, currRel := range current {
				for _, newRel := range newRels {
					if newRel.Key != currRel.Key {
						continue
					}
					// do not override selectDict
					newRel.SelectDict = currRel.SelectDict
				}
			}
			return newRels, nil
		})
	}

	return objects.New(
		r.s.Anytype().ObjectStore(),
		id,
		setDetailsNoContext,
		r.s.GetRelations,
		setOrAddRelations,
		r.s.ModifyExtraRelations,
		addOptionNoContext,
		r.s.CreateSmartBlock,
	), nil
}
