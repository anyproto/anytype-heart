package importer

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/common"
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

type importContext struct {
	ctx          context.Context
	origin       objectorigin.ObjectOrigin
	progress     process.Progress
	req          *pb.RpcObjectImportRequest
	convResponse *common.Response
	error        *common.ConvertError

	oldIDToNew           map[string]string
	createPayloads       map[string]treestorage.TreeStorageCreatePayload
	relationKeysToFormat map[domain.RelationKey]int32
}

func newImportContext(ctx context.Context, req *ImportRequest, resp *common.Response, origin objectorigin.ObjectOrigin, error *common.ConvertError) *importContext {
	if req == nil || resp == nil {
		return nil
	}
	if error == nil {
		error = common.NewError(req.Mode)
	}
	return &importContext{
		ctx:          ctx,
		origin:       origin,
		progress:     req.Progress,
		req:          req.RpcObjectImportRequest,
		convResponse: resp,
		error:        error,
	}
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
