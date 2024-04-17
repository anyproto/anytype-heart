package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/nameservice"
	"github.com/anyproto/anytype-heart/pb"
)

// NameServiceResolveName does a name lookup: somename.any -> info
func (mw *Middleware) NameServiceResolveName(ctx context.Context, req *pb.RpcNameServiceResolveNameRequest) *pb.RpcNameServiceResolveNameResponse {
	ns := getService[nameservice.Service](mw)
	return ns.NameServiceResolveName(ctx, req)
}

func (mw *Middleware) NameServiceResolveAnyId(ctx context.Context, req *pb.RpcNameServiceResolveAnyIdRequest) *pb.RpcNameServiceResolveAnyIdResponse {
	ns := getService[nameservice.Service](mw)
	return ns.NameServiceResolveAnyId(ctx, req)
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
	return ns.NameServiceUserAccountGet(ctx, req)
}
