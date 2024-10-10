package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/publish"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ObjectPublish(ctx context.Context, req *pb.RpcObjectPublishRequest) *pb.RpcObjectPublishResponse {
	publishService := getService[publish.Service](mw)

	res, _ := publishService.Publish(ctx, req.SpaceId, req.ObjectId)

	// code := mapErrorCode(err,
	// 	errToCode(nil, pb.RpcObjectPublishResponseError_NULL))
	// 		Error: &pb.RpcObjectPublishResponseError{
	// 		Code:        code,
	// 		Description: getErrorDescription(nil),
	// 	},

	r := &pb.RpcObjectPublishResponse{
		PublishCid:     res.Cid,
		PublishFileKey: res.Key,
	}

	return r
}
