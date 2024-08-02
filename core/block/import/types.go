package importer

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"

	types2 "github.com/anyproto/anytype-heart/core/block/import/common/types"
	_ "github.com/anyproto/anytype-heart/core/block/import/markdown"
	_ "github.com/anyproto/anytype-heart/core/block/import/pb"
	_ "github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
)

type ImportResponse struct {
	RootCollectionId string
	ProcessId        string
	ObjectsCount     int64
	Err              error
}

// Importer encapsulate logic with import
type Importer interface {
	app.Component
	Import(ctx context.Context,
		req *pb.RpcObjectImportRequest,
		origin objectorigin.ObjectOrigin,
		progress process.Progress,
	) *ImportResponse

	ListImports(req *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error)
	ImportWeb(ctx context.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error)
	// nolint: lll
	ValidateNotionToken(ctx context.Context, req *pb.RpcObjectImportNotionValidateTokenRequest) (pb.RpcObjectImportNotionValidateTokenResponseErrorCode, error)
	ImportSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest) ([]*types2.Snapshot, error)
}
