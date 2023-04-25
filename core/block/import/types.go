package importer

import (
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/markdown"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/pb"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/web"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/gogo/protobuf/types"
)

// Importer incapsulate logic with import
type Importer interface {
	app.Component
	Import(ctx *session.Context, req *pb.RpcObjectImportRequest) error
	ListImports(ctx *session.Context, req *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error)
	ImportWeb(ctx *session.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error)
	ImportUserData(ctx *session.Context, req *pb.RpcUserDataImportRequest) error
}

// Creator incapsulate logic with creation of given smartblocks
type Creator interface {
	//nolint:lll
	Create(ctx *session.Context, snapshot *converter.Snapshot, oldIDtoNew map[string]string, existing bool) (*types.Struct, error)
}

// IDGetter is interface for updating existing objects
type IDGetter interface {
	//nolint:lll
	Get(ctx *session.Context, cs *converter.Snapshot, sbType sb.SmartBlockType, updateExisting bool) (string, bool, error)
}
