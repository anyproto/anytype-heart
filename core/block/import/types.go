//go:generate mockgen -package importer -destination mock.go github.com/anyproto/anytype-heart/core/block/import Creator,IDGetter
package importer

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	_ "github.com/anyproto/anytype-heart/core/block/import/markdown"
	_ "github.com/anyproto/anytype-heart/core/block/import/pb"
	_ "github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Importer incapsulate logic with import
type Importer interface {
	app.Component
	Import(ctx *session.Context, req *pb.RpcObjectImportRequest) error
	ListImports(ctx *session.Context, req *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error)
	ImportWeb(ctx *session.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error)
	//nolint: lll
	ValidateNotionToken(ctx context.Context, req *pb.RpcObjectImportNotionValidateTokenRequest) (pb.RpcObjectImportNotionValidateTokenResponseErrorCode, error)
}

// Creator incapsulate logic with creation of given smartblocks
type Creator interface {
	//nolint:lll
	Create(ctx *session.Context, sn *converter.Snapshot, relations []*converter.Relation, oldIDtoNew map[string]string, createPayloads map[string]treestorage.TreeStorageCreatePayload, existing bool) (*types.Struct, string, error)
}

// IDGetter is interface for updating existing objects
type IDGetter interface {
	//nolint:lll
	Get(ctx *session.Context, cs *converter.Snapshot, sbType sb.SmartBlockType, createdTime time.Time, updateExisting bool) (string, bool, treestorage.TreeStorageCreatePayload, error)
}

// Updater is interface for updating existing objects
type Updater interface {
	//nolint: lll
	Update(ctx *session.Context, cs *model.SmartBlockSnapshotBase, relations []*converter.Relation, pageID string) (*types.Struct, []string, error)
}

// RelationCreator incapsulates logic for creation of RelationsIDToFormat
type RelationCreator interface {
	//nolint: lll
	ReplaceRelationBlock(ctx *session.Context, oldRelationBlocksToNew map[string]*model.Block, pageID string)
	//nolint: lll
	CreateRelations(ctx *session.Context, snapshot *model.SmartBlockSnapshotBase, pageID string, relations []*converter.Relation) ([]string, map[string]*model.Block, map[string]RelationsIDToFormat, error)
}
