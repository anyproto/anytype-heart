package importer

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"

	_ "github.com/anyproto/anytype-heart/core/block/import/markdown"
	_ "github.com/anyproto/anytype-heart/core/block/import/pb"
	_ "github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
)

// Importer encapsulate logic with import
type Importer interface {
	app.Component
	Import(ctx context.Context,
		req *pb.RpcObjectImportRequest,
		origin domain.ObjectOrigin,
		progress process.Progress,
	) (rootCollectionId string, processId string, err error)

	ListImports(req *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error)
	ImportWeb(ctx context.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error)
	// nolint: lll
	ValidateNotionToken(ctx context.Context, req *pb.RpcObjectImportNotionValidateTokenRequest) (pb.RpcObjectImportNotionValidateTokenResponseErrorCode, error)
}
