package core

import (
	"context"

	"github.com/anyproto/any-sync/net"

	"github.com/anyproto/anytype-heart/core/nameservice"
	"github.com/anyproto/anytype-heart/pb"
)

// NameServiceResolveName does a name lookup: somename.any -> info
func (mw *Middleware) NameServiceResolveName(ctx context.Context, req *pb.RpcNameServiceResolveNameRequest) *pb.RpcNameServiceResolveNameResponse {
	ns := getService[nameservice.Service](mw)
	out, err := ns.NameServiceResolveName(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(net.ErrUnableToConnect, pb.RpcNameServiceResolveNameResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := getErrorDescription(err)
		if code == pb.RpcNameServiceResolveNameResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcNameServiceResolveNameResponse{
			Error: &pb.RpcNameServiceResolveNameResponseError{
				Code:        code,
				Description: errStr,
			},
		}
	}

	return out
}

func (mw *Middleware) NameServiceResolveAnyId(ctx context.Context, req *pb.RpcNameServiceResolveAnyIdRequest) *pb.RpcNameServiceResolveAnyIdResponse {
	ns := getService[nameservice.Service](mw)

	out, err := ns.NameServiceResolveAnyId(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(net.ErrUnableToConnect, pb.RpcNameServiceResolveAnyIdResponseError_CAN_NOT_CONNECT),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := getErrorDescription(err)
		if code == pb.RpcNameServiceResolveAnyIdResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcNameServiceResolveAnyIdResponse{
			Error: &pb.RpcNameServiceResolveAnyIdResponseError{
				Code:        code,
				Description: errStr,
			},
		}
	}

	return out
}

func (mw *Middleware) NameServiceResolveSpaceId(ctx context.Context, req *pb.RpcNameServiceResolveSpaceIdRequest) *pb.RpcNameServiceResolveSpaceIdResponse {
	return &pb.RpcNameServiceResolveSpaceIdResponse{
		Error: &pb.RpcNameServiceResolveSpaceIdResponseError{
			Code:        pb.RpcNameServiceResolveSpaceIdResponseError_UNKNOWN_ERROR,
			Description: "TODO - not implemented yet",
		},
	}
}

func (mw *Middleware) NameServiceUserAccountGet(ctx context.Context, req *pb.RpcNameServiceUserAccountGetRequest) *pb.RpcNameServiceUserAccountGetResponse {
	ns := getService[nameservice.Service](mw)
	out, err := ns.NameServiceUserAccountGet(ctx, req)

	if err != nil {
		code := mapErrorCode(err,
			errToCode(net.ErrUnableToConnect, pb.RpcNameServiceUserAccountGetResponseError_CAN_NOT_CONNECT),
			errToCode(nameservice.ErrBadResolve, pb.RpcNameServiceUserAccountGetResponseError_BAD_NAME_RESOLVE),
		)

		// if client doesn't handle that error - let it show unlocalized string at least
		errStr := getErrorDescription(err)
		if code == pb.RpcNameServiceUserAccountGetResponseError_CAN_NOT_CONNECT {
			errStr = "please connect to the internet"
		}

		return &pb.RpcNameServiceUserAccountGetResponse{
			Error: &pb.RpcNameServiceUserAccountGetResponseError{
				Code:        code,
				Description: errStr,
			},
		}
	}

	return out
}
