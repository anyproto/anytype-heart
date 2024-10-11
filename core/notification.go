package core

import (
	"context"

	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (mw *Middleware) NotificationList(cctx context.Context, req *pb.RpcNotificationListRequest) *pb.RpcNotificationListResponse {
	response := func(code pb.RpcNotificationListResponseErrorCode, notificationsList []*model.Notification, err error) *pb.RpcNotificationListResponse {
		m := &pb.RpcNotificationListResponse{Error: &pb.RpcNotificationListResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Notifications = notificationsList
		}
		return m
	}
	notificationsList, err := getService[notifications.Notifications](mw).List(req.Limit, req.IncludeRead)

	if err != nil {
		return response(pb.RpcNotificationListResponseError_INTERNAL_ERROR, notificationsList, err)
	}
	return response(pb.RpcNotificationListResponseError_NULL, notificationsList, nil)
}

func (mw *Middleware) NotificationReply(cctx context.Context, req *pb.RpcNotificationReplyRequest) *pb.RpcNotificationReplyResponse {
	response := func(code pb.RpcNotificationReplyResponseErrorCode, err error) *pb.RpcNotificationReplyResponse {
		m := &pb.RpcNotificationReplyResponse{Error: &pb.RpcNotificationReplyResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := getService[notifications.Notifications](mw).Reply(req.Ids, req.ActionType)

	if err != nil {
		return response(pb.RpcNotificationReplyResponseError_INTERNAL_ERROR, err)
	}
	return response(pb.RpcNotificationReplyResponseError_NULL, nil)
}

func (mw *Middleware) NotificationTest(cctx context.Context, req *pb.RpcNotificationTestRequest) *pb.RpcNotificationTestResponse {
	response := func(code pb.RpcNotificationTestResponseErrorCode, err error) *pb.RpcNotificationTestResponse {
		m := &pb.RpcNotificationTestResponse{Error: &pb.RpcNotificationTestResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := getService[notifications.Notifications](mw).CreateAndSend(&model.Notification{
		Id:      uuid.New().String(),
		Status:  model.Notification_Created,
		IsLocal: true,
		Payload: &model.NotificationPayloadOfTest{Test: &model.NotificationTest{}},
	})

	if err != nil {
		return response(pb.RpcNotificationTestResponseError_INTERNAL_ERROR, err)
	}
	return response(pb.RpcNotificationTestResponseError_NULL, nil)
}
