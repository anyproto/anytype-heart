package core

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/device"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) DeviceNetworkStateSet(cctx context.Context, req *pb.RpcDeviceNetworkStateSetRequest) *pb.RpcDeviceNetworkStateSetResponse {
	response := func(code pb.RpcDeviceNetworkStateSetResponseErrorCode, err error) *pb.RpcDeviceNetworkStateSetResponse {
		m := &pb.RpcDeviceNetworkStateSetResponse{Error: &pb.RpcDeviceNetworkStateSetResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	mwApp := mw.GetApp()
	if mwApp == nil {
		return response(pb.RpcDeviceNetworkStateSetResponseError_INTERNAL_ERROR, ErrNotLoggedIn)
	}
	app.MustComponent[device.NetworkState](mwApp).SetNetworkState(req.DeviceNetworkType)
	return response(pb.RpcDeviceNetworkStateSetResponseError_NULL, nil)
}
