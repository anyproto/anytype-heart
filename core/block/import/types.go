package importer

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/markdown"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/notion"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/pb"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
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
	Create(ctx *session.Context, cs *model.SmartBlockSnapshotBase, pageID string, sbType smartblock.SmartBlockType, updateExisting bool) (*types.Struct, error)
}

// Updater is interface for updating existing objects
type Updater interface {
	Update(ctx *session.Context, cs *model.SmartBlockSnapshotBase, pageID string) (*types.Struct, error)
}

// RelationCreator incapsulates logic for creation of relations
type RelationCreator interface {
	Create(ctx *session.Context, snapshot *model.SmartBlockSnapshotBase, pageID string) ([]string, error)
}
