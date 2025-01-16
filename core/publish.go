package core

import (
	"context"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/publish"
	"github.com/anyproto/anytype-heart/pb"
)

// rpc PublishingCreate (anytype.Rpc.Publishing.Create.Request) returns (anytype.Rpc.Publishing.Create.Response)
// rpc PublishingRemove (anytype.Rpc.Publishing.Remove.Request) returns (anytype.Rpc.Publishing.Remove.Response)
// rpc PublishingList (anytype.Rpc.Publishing.List.Request) returns (anytype.Rpc.Publishing.List.Response)
// rpc PublishingResolveUri (anytype.Rpc.Publishing.ResolveUri.Request) returns (anytype.Rpc.Publishing.ResolveUri.Response)
// rpc PublishingGetStatus (anytype.Rpc.Publishing.GetStatus.Request) returns (anytype.Rpc.Publishing.GetStatus.Response)

func (mw *Middleware) PublishingCreate(ctx context.Context, req *pb.RpcPublishingCreateRequest) *pb.RpcPublishingCreateResponse {
	log.Error("PublishingCreate called", zap.String("objectId", req.ObjectId))
	publishService := mustService[publish.Service](mw)

	res, err := publishService.Publish(ctx, req.SpaceId, req.ObjectId, req.Uri, req.JoinSpace)
	log.Error("PublishingCreate called", zap.String("objectId", req.ObjectId))
	code := mapErrorCode(err,
		errToCode(nil, pb.RpcPublishingCreateResponseError_NULL),
		errToCode(err, pb.RpcPublishingCreateResponseError_UNKNOWN_ERROR),
		errToCode(publish.ErrLimitExceeded, pb.RpcPublishingCreateResponseError_LIMIT_EXCEEDED))

	r := &pb.RpcPublishingCreateResponse{
		Error: &pb.RpcPublishingCreateResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		Uri: res.Cid,
	}

	return r
}

func (mw *Middleware) PublishingRemove(ctx context.Context, req *pb.RpcPublishingRemoveRequest) *pb.RpcPublishingRemoveResponse {
	publishService := mustService[publish.Service](mw)

	err := publishService.Unpublish(ctx, req.SpaceId, req.ObjectId)
	code := mapErrorCode(nil,
		errToCode(err, pb.RpcPublishingRemoveResponseError_NULL))

	r := &pb.RpcPublishingRemoveResponse{
		Error: &pb.RpcPublishingRemoveResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
	return r
}

func (mw *Middleware) PublishingList(ctx context.Context, req *pb.RpcPublishingListRequest) *pb.RpcPublishingListResponse {
	publishService := mustService[publish.Service](mw)

	publishes, err := publishService.PublishList(ctx, req.SpaceId)
	code := mapErrorCode(nil,
		errToCode(err, pb.RpcPublishingListResponseError_NULL))

	r := &pb.RpcPublishingListResponse{
		Error: &pb.RpcPublishingListResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		Publishes: publishes,
	}
	return r
}

func (mw *Middleware) PublishingResolveUri(ctx context.Context, req *pb.RpcPublishingResolveUriRequest) *pb.RpcPublishingResolveUriResponse {
	publishService := mustService[publish.Service](mw)

	publish, err := publishService.ResolveUri(ctx, req.Uri)
	code := mapErrorCode(nil,
		errToCode(err, pb.RpcPublishingResolveUriResponseError_NULL))

	r := &pb.RpcPublishingResolveUriResponse{
		Error: &pb.RpcPublishingResolveUriResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		Publish: publish,
	}
	return r
}

func (mw *Middleware) PublishingGetStatus(ctx context.Context, req *pb.RpcPublishingGetStatusRequest) *pb.RpcPublishingGetStatusResponse {
	publishService := mustService[publish.Service](mw)

	publish, err := publishService.GetStatus(ctx, req.SpaceId, req.ObjectId)
	code := mapErrorCode(nil,
		errToCode(err, pb.RpcPublishingGetStatusResponseError_NULL))

	r := &pb.RpcPublishingGetStatusResponse{
		Error: &pb.RpcPublishingGetStatusResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		Publish: publish,
	}
	return r
}
