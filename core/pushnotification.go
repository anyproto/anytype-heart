package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/pushnotification"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) PushNotificationRegisterToken(cctx context.Context, req *pb.RpcPushNotificationRegisterTokenRequest) *pb.RpcPushNotificationRegisterTokenResponse {
	response := func(code pb.RpcPushNotificationRegisterTokenResponseErrorCode, err error) *pb.RpcPushNotificationRegisterTokenResponse {
		m := &pb.RpcPushNotificationRegisterTokenResponse{Error: &pb.RpcPushNotificationRegisterTokenResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[pushnotification.Service](mw).RegisterToken(cctx, req)

	if err != nil {
		return response(pb.RpcPushNotificationRegisterTokenResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcPushNotificationRegisterTokenResponseError_NULL, nil)
}
