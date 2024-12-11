package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/publish"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) PublishObject(ctx context.Context, req *pb.RpcObjectPublishRequest) *pb.RpcObjectPublishResponse {
	publishService := getService[publish.Service](mw)

	res, err := publishService.Publish(ctx, req.SpaceId, req.ObjectId)
	code := mapErrorCode(err,
		errToCode(nil, pb.RpcObjectPublishResponseError_NULL))

	r := &pb.RpcObjectPublishResponse{
		Error: &pb.RpcObjectPublishResponseError{
			Code:        code,
			Description: getErrorDescription(nil),
		},

		PublishCid:     res.Cid,
		PublishFileKey: res.Key,
	}

	return r
}

func (mw *Middleware) UnpublishObject(ctx context.Context, req *pb.RpcObjectPublishRequest) *pb.RpcObjectPublishResponse {
	return r
}

func (mw *Middleware) ListPublishings(ctx context.Context, req *pb.RpcObjectPublishRequest) *pb.RpcObjectPublishResponse {
	return r
}

func (mw *Middleware) ResolvePublishUri(ctx context.Context, req *pb.RpcObjectPublishRequest) *pb.RpcObjectPublishResponse {
	return r
}

func (mw *Middleware) GetPublishStatus(ctx context.Context, req *pb.RpcObjectPublishRequest) *pb.RpcObjectPublishResponse {
	return r
}
