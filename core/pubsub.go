package core

import (
	"context"

	"github.com/anyproto/any-sync/commonspace/pubsub/pubsubproto"

	"github.com/anyproto/anytype-heart/core/pubsub"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) PubsubPublish(cctx context.Context, req *pb.RpcPubsubPublishRequest) *pb.RpcPubsubPublishResponse {
	err := mustService[pubsub.Service](mw).Publish(cctx, req.SpaceId, req.Topic, req.Payload)
	code := mapErrorCode(err,
		errToCode(pubsubproto.ErrInvalidTopic, pb.RpcPubsubPublishResponseError_BAD_INPUT),
		errToCode(pubsubproto.ErrInvalidMessage, pb.RpcPubsubPublishResponseError_BAD_INPUT),
		errToCode(pubsubproto.ErrTopicNotOwned, pb.RpcPubsubPublishResponseError_BAD_INPUT),
	)
	return &pb.RpcPubsubPublishResponse{
		Error: &pb.RpcPubsubPublishResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) PubsubSubscribe(cctx context.Context, req *pb.RpcPubsubSubscribeRequest) *pb.RpcPubsubSubscribeResponse {
	subId, err := mustService[pubsub.Service](mw).Subscribe(req.SpaceId, req.Topics, req.SubId)
	code := mapErrorCode(err,
		errToCode(pubsubproto.ErrInvalidTopic, pb.RpcPubsubSubscribeResponseError_BAD_INPUT),
		errToCode(pubsubproto.ErrTooManyTopics, pb.RpcPubsubSubscribeResponseError_BAD_INPUT),
		errToCode(pubsub.ErrEmptyTopics, pb.RpcPubsubSubscribeResponseError_BAD_INPUT),
	)
	return &pb.RpcPubsubSubscribeResponse{
		SubId: subId,
		Error: &pb.RpcPubsubSubscribeResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) PubsubUnsubscribe(cctx context.Context, req *pb.RpcPubsubUnsubscribeRequest) *pb.RpcPubsubUnsubscribeResponse {
	err := mustService[pubsub.Service](mw).Unsubscribe(req.SubId)
	code := mapErrorCode(err,
		errToCode(pubsub.ErrSubscriptionNotFound, pb.RpcPubsubUnsubscribeResponseError_BAD_INPUT),
	)
	return &pb.RpcPubsubUnsubscribeResponse{
		Error: &pb.RpcPubsubUnsubscribeResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}
