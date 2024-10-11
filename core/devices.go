package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/device"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (mw *Middleware) DeviceSetName(cctx context.Context, req *pb.RpcDeviceSetNameRequest) *pb.RpcDeviceSetNameResponse {
	response := func(code pb.RpcDeviceSetNameResponseErrorCode, err error) *pb.RpcDeviceSetNameResponse {
		m := &pb.RpcDeviceSetNameResponse{Error: &pb.RpcDeviceSetNameResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := getService[device.Service](mw).UpdateName(cctx, req.DeviceId, req.Name)
	if err != nil {
		return response(pb.RpcDeviceSetNameResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcDeviceSetNameResponseError_NULL, nil)
}

func (mw *Middleware) DeviceList(cctx context.Context, _ *pb.RpcDeviceListRequest) *pb.RpcDeviceListResponse {
	response := func(code pb.RpcDeviceListResponseErrorCode, devices []*model.DeviceInfo, err error) *pb.RpcDeviceListResponse {
		m := &pb.RpcDeviceListResponse{Error: &pb.RpcDeviceListResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		m.Devices = devices
		return m
	}
	devices, err := getService[device.Service](mw).ListDevices(cctx)
	if err != nil {
		return response(pb.RpcDeviceListResponseError_UNKNOWN_ERROR, devices, err)
	}
	return response(pb.RpcDeviceListResponseError_NULL, devices, nil)
}
