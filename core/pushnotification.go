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
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcPushNotificationSetSpaceModeResponseErrorCode, err error) *pb.RpcPushNotificationSetSpaceModeResponse {
		m := &pb.RpcPushNotificationSetSpaceModeResponse{
			Error: &pb.RpcPushNotificationSetSpaceModeResponseError{Code: code},
			Event: mw.getResponseEvent(ctx),
		}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[space.Service](mw).TechSpace().DoSpaceView(cctx, req.SpaceId, func(spaceView techspace.SpaceView) error {
		return spaceView.SetPushNotificationMode(ctx, req.Mode)
	})
	if err != nil {
		return response(pb.RpcPushNotificationSetSpaceModeResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcPushNotificationSetSpaceModeResponseError_NULL, nil)
}

func (mw *Middleware) PushNotificationSetForceModeIds(cctx context.Context, req *pb.RpcPushNotificationSetForceModeIdsRequest) *pb.RpcPushNotificationSetForceModeIdsResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcPushNotificationSetForceModeIdsResponseErrorCode, err error) *pb.RpcPushNotificationSetForceModeIdsResponse {
		m := &pb.RpcPushNotificationSetForceModeIdsResponse{
			Error: &pb.RpcPushNotificationSetForceModeIdsResponseError{Code: code},
			Event: mw.getResponseEvent(ctx),
		}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[space.Service](mw).TechSpace().DoSpaceView(cctx, req.SpaceId, func(spaceView techspace.SpaceView) error {
		return spaceView.SetPushNotificationForceModeIds(ctx, req.ChatIds, req.Mode)
	})
	if err != nil {
		return response(pb.RpcPushNotificationSetForceModeIdsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcPushNotificationSetForceModeIdsResponseError_NULL, nil)
}

func (mw *Middleware) PushNotificationResetIds(cctx context.Context, req *pb.RpcPushNotificationResetIdsRequest) *pb.RpcPushNotificationResetIdsResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcPushNotificationResetIdsResponseErrorCode, err error) *pb.RpcPushNotificationResetIdsResponse {
		m := &pb.RpcPushNotificationResetIdsResponse{
			Error: &pb.RpcPushNotificationResetIdsResponseError{Code: code},
			Event: mw.getResponseEvent(ctx),
		}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[space.Service](mw).TechSpace().DoSpaceView(cctx, req.SpaceId, func(spaceView techspace.SpaceView) error {
		return spaceView.ResetPushNotificationIds(ctx, req.ChatIds)
	})
	if err != nil {
		return response(pb.RpcPushNotificationResetIdsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcPushNotificationResetIdsResponseError_NULL, nil)
}
