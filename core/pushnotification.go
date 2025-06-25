package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/pushnotification"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/techspace"
)

func (mw *Middleware) PushNotificationRegisterToken(cctx context.Context, req *pb.RpcPushNotificationRegisterTokenRequest) *pb.RpcPushNotificationRegisterTokenResponse {
	response := func(code pb.RpcPushNotificationRegisterTokenResponseErrorCode, err error) *pb.RpcPushNotificationRegisterTokenResponse {
		m := &pb.RpcPushNotificationRegisterTokenResponse{Error: &pb.RpcPushNotificationRegisterTokenResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	mustService[pushnotification.Service](mw).RegisterToken(req)

	return response(pb.RpcPushNotificationRegisterTokenResponseError_NULL, nil)
}

func (mw *Middleware) PushNotificationSetSpaceMode(cctx context.Context, req *pb.RpcPushNotificationSetSpaceModeRequest) *pb.RpcPushNotificationSetSpaceModeResponse {
	response := func(code pb.RpcPushNotificationSetSpaceModeResponseErrorCode, err error) *pb.RpcPushNotificationSetSpaceModeResponse {
		m := &pb.RpcPushNotificationSetSpaceModeResponse{Error: &pb.RpcPushNotificationSetSpaceModeResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	ctx := mw.newContext(cctx)
	err := mustService[space.Service](mw).TechSpace().DoSpaceView(cctx, req.SpaceId, func(spaceView techspace.SpaceView) error {
		return spaceView.SetPushNotificationMode(ctx, req.Mode)
	})
	if err != nil {
		return response(pb.RpcPushNotificationSetSpaceModeResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcPushNotificationSetSpaceModeResponseError_NULL, nil)
}
