package importer

import (
	"context"

	"github.com/anytypeio/any-sync/app"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/markdown"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/newinfra"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/pb"
	_ "github.com/anytypeio/go-anytype-middleware/core/block/import/web"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

// Importer incapsulate logic with import
type Importer interface {
	app.Component
	Import(ctx *session.Context, req *pb.RpcObjectImportRequest) error
	ListImports(ctx *session.Context, req *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error)
	ImportWeb(ctx *session.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error)
	//nolint: lll
	ValidateNotionToken(ctx context.Context, req *pb.RpcObjectImportNotionValidateTokenRequest) pb.RpcObjectImportNotionValidateTokenResponseErrorCode
}

// Creator incapsulate logic with creation of given smartblocks
type Creator interface {
	//nolint:lll
	Create(ctx *session.Context, snapshot *converter.Snapshot, relations []*converter.Relation, oldIDtoNew map[string]string, existing bool) (*types.Struct, error)
}

// IDGetter is interface for updating existing objects
type IDGetter interface {
	//nolint:lll
	Get(ctx *session.Context, cs *converter.Snapshot, sbType sb.SmartBlockType, updateExisting bool) (string, bool, error)
}

// Updater is interface for updating existing objects
type Updater interface {
	//nolint: lll
	Update(ctx *session.Context, cs *model.SmartBlockSnapshotBase, relations []*converter.Relation, pageID string) (*types.Struct, []string, error)
}

// RelationCreator incapsulates logic for creation of relations
type RelationCreator interface {
	//nolint: lll
	ReplaceRelationBlock(ctx *session.Context, oldRelationBlocksToNew map[string]*model.Block, pageID string)
	//nolint: lll
	CreateRelations(ctx *session.Context, snapshot *model.SmartBlockSnapshotBase, pageID string, relations []*converter.Relation) ([]string, map[string]*model.Block, error)
}
