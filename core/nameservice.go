package core

import (
	"context"

	"github.com/anyproto/any-sync/nameservice/nameserviceclient"
	proto "github.com/anyproto/any-sync/nameservice/nameserviceproto"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
)

// NameServiceResolveName does a name lookup: somename.any -> info
func (mw *Middleware) NameServiceResolveName(ctx context.Context, req *pb.RpcNameServiceResolveNameRequest) *pb.RpcNameServiceResolveNameResponse {
	ns := getService[nameserviceclient.AnyNsClientService](mw)

	var in proto.NameAvailableRequest
	in.FullName = req.FullName

	nar, err := ns.IsNameAvailable(ctx, &in)
	if err != nil {
		return &pb.RpcNameServiceResolveNameResponse{
			Error: &pb.RpcNameServiceResolveNameResponseError{
				// we don't map error codes here
				Code:        pb.RpcNameServiceResolveNameResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	// Return the response
	var out pb.RpcNameServiceResolveNameResponse
	out.Available = nar.Available
	out.OwnerAnyAddress = nar.OwnerAnyAddress
	// EOA is onwer of -> SCW is owner of -> name
	out.OwnerEthAddress = nar.OwnerEthAddress
	out.OwnerScwEthAddress = nar.OwnerScwEthAddress
	out.SpaceId = nar.SpaceId
	out.NameExpires = nar.NameExpires

	return &out
}

func (mw *Middleware) NameServiceResolveAnyId(ctx context.Context, req *pb.RpcNameServiceResolveAnyIdRequest) *pb.RpcNameServiceResolveAnyIdResponse {
	// Get name service object that connects to the remote "namingNode"
	// in order for that to work, we need to have a "namingNode" node in the nodes section of the config
	ns := getService[nameserviceclient.AnyNsClientService](mw)

	var in proto.NameByAnyIdRequest
	in.AnyAddress = req.AnyId

	nar, err := ns.GetNameByAnyId(ctx, &in)
	if err != nil {
		return &pb.RpcNameServiceResolveAnyIdResponse{
			Error: &pb.RpcNameServiceResolveAnyIdResponseError{
				// we don't map error codes here
				Code:        pb.RpcNameServiceResolveAnyIdResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	// Return the response
	var out pb.RpcNameServiceResolveAnyIdResponse
	out.Found = nar.Found
	out.FullName = nar.Name

	return &out
}

func (mw *Middleware) NameServiceResolveSpaceId(ctx context.Context, req *pb.RpcNameServiceResolveSpaceIdRequest) *pb.RpcNameServiceResolveSpaceIdResponse {
	// TODO: implement
	// TODO: test

	return &pb.RpcNameServiceResolveSpaceIdResponse{
		Error: &pb.RpcNameServiceResolveSpaceIdResponseError{
			Code:        pb.RpcNameServiceResolveSpaceIdResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) NameServiceUserAccountGet(ctx context.Context, req *pb.RpcNameServiceUserAccountGetRequest) *pb.RpcNameServiceUserAccountGetResponse {
	// 1 - get name service object that connects to the remote "namingNode"
	// in order for that to work, we need to have a "namingNode" node in the nodes section of the config
	ns := getService[nameserviceclient.AnyNsClientService](mw)

	// 2 - get user's ETH address from the wallet
	w := getService[wallet.Wallet](mw)

	// 3 - get user's account info
	//
	// when AccountAbstraction is used to deploy a smart contract wallet
	// then name is really owned by this SCW, but owner of this SCW is
	// EOA that was used to sign transaction
	//
	// EOA (w.GetAccountEthAddress()) -> SCW (ua.OwnerSmartContracWalletAddress) -> name
	var guar proto.GetUserAccountRequest
	guar.OwnerEthAddress = w.GetAccountEthAddress().Hex()

	ua, err := ns.GetUserAccount(ctx, &guar)
	if err != nil {
		return &pb.RpcNameServiceUserAccountGetResponse{
			Error: &pb.RpcNameServiceUserAccountGetResponseError{
				Code:        pb.RpcNameServiceUserAccountGetResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	// 4 - check if any name is attached to the account (reverse resolve the name)
	var in proto.NameByAddressRequest

	// NOTE: we are passing here SCW address, not initial ETH address!
	// read comment about SCW above please
	in.OwnerScwEthAddress = ua.OwnerSmartContracWalletAddress

	nar, err := ns.GetNameByAddress(ctx, &in)
	if err != nil {
		return &pb.RpcNameServiceUserAccountGetResponse{
			Error: &pb.RpcNameServiceUserAccountGetResponseError{
				// we don't map error codes here
				Code:        pb.RpcNameServiceUserAccountGetResponseError_BAD_NAME_RESOLVE,
				Description: err.Error(),
			},
		}
	}

	// Return the response
	var out pb.RpcNameServiceUserAccountGetResponse
	out.NamesCountLeft = ua.NamesCountLeft
	out.OperationsCountLeft = ua.OperationsCountLeft
	// not checking nar.Found here, no need
	out.AnyNameAttached = nar.Name

	return &out
}
