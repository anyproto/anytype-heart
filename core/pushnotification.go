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

func (mw *Middleware) PushNotificationAddMuteIds(cctx context.Context, req *pb.RpcPushNotificationAddMuteIdsRequest) *pb.RpcPushNotificationAddMuteIdsResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcPushNotificationAddMuteIdsResponseErrorCode, err error) *pb.RpcPushNotificationAddMuteIdsResponse {
		m := &pb.RpcPushNotificationAddMuteIdsResponse{
			Error: &pb.RpcPushNotificationAddMuteIdsResponseError{Code: code},
			Event: mw.getResponseEvent(ctx),
		}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[space.Service](mw).TechSpace().DoSpaceView(cctx, req.SpaceId, func(spaceView techspace.SpaceView) error {
		return spaceView.AddPushNotificationMuteIds(ctx, req.ChatIds)
	})
	if err != nil {
		return response(pb.RpcPushNotificationAddMuteIdsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcPushNotificationAddMuteIdsResponseError_NULL, nil)
}

func (mw *Middleware) PushNotificationAddMentionIds(cctx context.Context, req *pb.RpcPushNotificationAddMentionIdsRequest) *pb.RpcPushNotificationAddMentionIdsResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcPushNotificationAddMentionIdsResponseErrorCode, err error) *pb.RpcPushNotificationAddMentionIdsResponse {
		m := &pb.RpcPushNotificationAddMentionIdsResponse{
			Error: &pb.RpcPushNotificationAddMentionIdsResponseError{Code: code},
			Event: mw.getResponseEvent(ctx),
		}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[space.Service](mw).TechSpace().DoSpaceView(cctx, req.SpaceId, func(spaceView techspace.SpaceView) error {
		return spaceView.AddPushNotificationMentionIds(ctx, req.ChatIds)
	})
	if err != nil {
		return response(pb.RpcPushNotificationAddMentionIdsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcPushNotificationAddMentionIdsResponseError_NULL, nil)
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
