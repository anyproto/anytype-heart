//go:generate mockgen -package importer -destination mock.go github.com/anyproto/anytype-heart/core/block/import Creator,IDGetter
package importer

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"

	_ "github.com/anyproto/anytype-heart/core/block/import/markdown"
	_ "github.com/anyproto/anytype-heart/core/block/import/pb"
	_ "github.com/anyproto/anytype-heart/core/block/import/web"
)

// Importer incapsulate logic with import
type Importer interface {
	app.Component
	Import(ctx context.Context, req *pb.RpcObjectImportRequest) error
	ListImports(ctx session.Context, req *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error)
	ImportWeb(ctx context.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error)
	// nolint: lll
	ValidateNotionToken(ctx context.Context, req *pb.RpcObjectImportNotionValidateTokenRequest) (pb.RpcObjectImportNotionValidateTokenResponseErrorCode, error)
}

// Creator incapsulate logic with creation of given smartblocks
type Creator interface {
	//nolint:lll
	Create(ctx context.Context, spaceID string, sn *converter.Snapshot, oldIDtoNew map[string]string, createPayloads map[string]treestorage.TreeStorageCreatePayload, filesIDs []string) (*types.Struct, string, error)
}

// IDGetter is interface for updating existing objects
type IDGetter interface {
	//nolint:lll
	Get(ctx context.Context, spaceID string, cs *converter.Snapshot, sbType sb.SmartBlockType, createdTime time.Time, updateExisting bool, oldIDToNew map[string]string) (string, treestorage.TreeStorageCreatePayload, error)
}
