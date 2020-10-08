package database

import (
	"github.com/anytypeio/go-anytype-middleware/core/anytype"
	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/pb"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/gogo/protobuf/types"
)

type Ctrl interface {
	Anytype() anytype.Service
	SetDetails(req pb.RpcBlockSetDetailsRequest) error
	CreateSmartBlock(sbType coresb.SmartBlockType, details *types.Struct, objectTypes []string, relations []*pbrelation.Relation) (id string, err error)
	GetObjectType(url string) (objectType *pbrelation.ObjectType, err error)
	UpdateRelations(id string, relations []*pbrelation.Relation) (err error)
	AddRelations(id string, relations []*pbrelation.Relation) (relationsWithKeys []*pbrelation.Relation, err error)
	RemoveRelations(id string, relationKeys []string) (err error)
	CreateSet(objType *pbrelation.ObjectType, name, icon string) (id string, err error)

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
	return objects.New(r.s.Anytype().ObjectStore(), id, r.s.SetDetails, r.s.CreateSmartBlock), nil
}
