package core

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ApiStartServer(cctx context.Context, req *pb.RpcApiStartServerRequest) *pb.RpcApiStartServerResponse {
	response := func(err error) *pb.RpcApiStartServerResponse {
		m := &pb.RpcApiStartServerResponse{
			Error: &pb.RpcApiStartServerResponseError{
				Code: pb.RpcApiStartServerResponseError_NULL,
			},
		}
		if err != nil {
			m.Error.Code = mapErrorCode(err,
				errToCode(api.ErrPortAlreadyUsed, pb.RpcApiStartServerResponseError_PORT_ALREADY_USED),
				errToCode(api.ErrServerAlreadyStarted, pb.RpcApiStartServerResponseError_SERVER_ALREADY_STARTED))
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}

	apiService := mw.applicationService.GetApp().Component(api.CName).(api.Api)
	if apiService == nil {
		return response(fmt.Errorf("node not started"))
	}

	err := apiService.Start()
	return response(err)
}

func (mw *Middleware) ApiStopServer(cctx context.Context, req *pb.RpcApiStopServerRequest) *pb.RpcApiStopServerResponse {
	response := func(err error) *pb.RpcApiStopServerResponse {
		m := &pb.RpcApiStopServerResponse{
			Error: &pb.RpcApiStopServerResponseError{
				Code: pb.RpcApiStopServerResponseError_NULL,
			},
		}
		if err != nil {
			m.Error.Code = mapErrorCode(err, errToCode(api.ErrServerNotStarted, pb.RpcApiStopServerResponseError_SERVER_NOT_STARTED))
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}

	apiService := mw.applicationService.GetApp().Component(api.CName).(api.Api)
	if apiService == nil {
		return response(fmt.Errorf("node not started"))
	}

	err := apiService.Stop()
	return response(err)
}
