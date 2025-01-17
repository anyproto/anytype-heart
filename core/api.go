package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/api"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ApiStartServer(ctx context.Context, req *pb.RpcApiStartServerRequest) *pb.RpcApiStartServerResponse {
	apiService := mustService[api.Service](mw)

	err := apiService.Start()
	code := mapErrorCode(err,
		errToCode(api.ErrPortAlreadyUsed, pb.RpcApiStartServerResponseError_PORT_ALREADY_USED),
		errToCode(api.ErrServerAlreadyStarted, pb.RpcApiStartServerResponseError_SERVER_ALREADY_STARTED))

	r := &pb.RpcApiStartServerResponse{
		Error: &pb.RpcApiStartServerResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
	return r
}

func (mw *Middleware) ApiStopServer(ctx context.Context, req *pb.RpcApiStopServerRequest) *pb.RpcApiStopServerResponse {
	apiService := mustService[api.Service](mw)

	err := apiService.Stop()
	code := mapErrorCode(nil,
		errToCode(api.ErrServerNotStarted, pb.RpcApiStopServerResponseError_SERVER_NOT_STARTED))

	r := &pb.RpcApiStopServerResponse{
		Error: &pb.RpcApiStopServerResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
	return r
}
