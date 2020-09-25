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
	CreateSmartBlock(sbType coresb.SmartBlockType, details *types.Struct) (pageId string, err error)
	GetObjectType(url string) (objectType *pbrelation.ObjectType, err error)
}

type Router interface {
	Get(id string) (database.Database, error)
}

func New(s Ctrl) Router {
	return &router{s: s}
}

type router struct{ s Ctrl }

func (r router) Get(id string) (database.Database, error) {
	// compatibility with older versions
	if id == "pages" {
		id = "https://anytype.io/schemas/object/bundled/pages"
	}

	return objects.New(r.s.Anytype().ObjectStore(), id, r.s.SetDetails, r.s.CreateSmartBlock), nil
}
