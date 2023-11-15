package core

import (
	"context"

	aa "github.com/anyproto/any-ns-node/pb/anyns_api"

	"github.com/anyproto/anytype-heart/pb"
)

// NameServiceResolveName does a name lookup: somename.any -> info
func (mw *Middleware) NameServiceResolveName(ctx context.Context, req *pb.RpcNameServiceResolveNameRequest) *pb.RpcNameServiceResolveNameResponse {
	// Get name service object that connects to the remote "namingNode"
	// in order for that to work, we need to have a "namingNode" node in the nodes section of the config
	// see https://github.com/anyproto/any-ns-node/blob/main/etc/ for example
	ns, err := mw.getNameService()

	if err != nil {
		return &pb.RpcNameServiceResolveNameResponse{
			Error: &pb.RpcNameServiceResolveNameResponseError{
				// we don't map error codes here
				Code:        -1,
				Description: err.Error(),
			},
		}
	}

	var in aa.NameAvailableRequest
	in.FullName = req.FullName

	nar, err := ns.IsNameAvailable(ctx, &in)
	if err != nil {
		return &pb.RpcNameServiceResolveNameResponse{
			Error: &pb.RpcNameServiceResolveNameResponseError{
				// we don't map error codes here
				Code:        -1,
				Description: err.Error(),
			},
		}
	}

	// Return the response
	var out pb.RpcNameServiceResolveNameResponse
	out.Available = nar.Available
	out.OwnerAnyAddress = nar.OwnerAnyAddress
	out.OwnerEthAddress = nar.OwnerEthAddress
	out.SpaceId = nar.SpaceId
	out.NameExpires = nar.NameExpires

	return &out
}

// NameServiceReverseResolveName does a reverse name lookup: address -> somename.any
func (mw *Middleware) NameServiceReverseResolveName(ctx context.Context, req *pb.RpcNameServiceReverseResolveNameRequest) *pb.RpcNameServiceReverseResolveNameResponse {
	// Get name service object that connects to the remote "namingNode"
	// in order for that to work, we need to have a "namingNode" node in the nodes section of the config
	ns, err := mw.getNameService()

	if err != nil {
		return &pb.RpcNameServiceReverseResolveNameResponse{
			Error: &pb.RpcNameServiceReverseResolveNameResponseError{
				// we don't map error codes here
				Code:        -1,
				Description: err.Error(),
			},
		}
	}

	var in aa.NameByAddressRequest
	in.OwnerEthAddress = req.OwnerEthAddress

	nar, err := ns.GetNameByAddress(ctx, &in)
	if err != nil {
		return &pb.RpcNameServiceReverseResolveNameResponse{
			Error: &pb.RpcNameServiceReverseResolveNameResponseError{
				// we don't map error codes here
				Code:        -1,
				Description: err.Error(),
			},
		}
	}

	// Return the response
	var out pb.RpcNameServiceReverseResolveNameResponse
	out.Found = nar.Found
	out.Name = nar.Name

	return &out
}
