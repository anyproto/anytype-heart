package importer

import (
	"context"

	"github.com/anyproto/any-sync/app"

	_ "github.com/anyproto/anytype-heart/core/block/import/markdown"
	_ "github.com/anyproto/anytype-heart/core/block/import/pb"
	_ "github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ImportRequest struct {
	*pb.RpcObjectImportRequest
	Origin           objectorigin.ObjectOrigin
	Progress         process.Progress
	SendNotification bool
	IsSync           bool
}

type ImportResponse struct {
	RootCollectionId string
	RootWidgetLayout model.BlockContentWidgetLayout
	ProcessId        string
	ObjectsCount     int64
	Err              error
}

// Importer encapsulate logic with import
type Importer interface {
	app.Component
	Import(ctx context.Context, importRequest *ImportRequest) *ImportResponse

	ListImports(req *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error)
	ImportWeb(ctx context.Context, req *ImportRequest) (string, *domain.Details, error)
	// nolint: lll
	ValidateNotionToken(ctx context.Context, req *pb.RpcObjectImportNotionValidateTokenRequest) (pb.RpcObjectImportNotionValidateTokenResponseErrorCode, error)
}
