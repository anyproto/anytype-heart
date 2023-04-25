package importer

import (
	"github.com/anytypeio/any-sync/app"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/markdown"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/pb"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/web"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/gogo/protobuf/types"
)

// Importer incapsulate logic with import
type Importer interface {
	app.Component
	Import(ctx *session.Context, req *pb.RpcObjectImportRequest) error
	ListImports(ctx *session.Context, req *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error)
	ImportWeb(ctx *session.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error)
}

// Creator incapsulate logic with creation of given smartblocks
type Creator interface {
	Create(ctx *session.Context, cs *model.SmartBlockSnapshotBase, pageID string, updateExisting bool) (*types.Struct, error)
}

// Updater is interface for updating existing objects
type Updater interface {
	Update(ctx *session.Context, cs *model.SmartBlockSnapshotBase, pageID string) (*types.Struct, error)
}
