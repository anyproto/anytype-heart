package core

import (
	"context"

	proto "github.com/anyproto/any-sync/nameservice/nameserviceproto"

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
				Code:        pb.RpcNameServiceResolveNameResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

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

func (mw *Middleware) NameServiceResolveAnyID(ctx context.Context, req *pb.RpcNameServiceResolveAnyIDRequest) *pb.RpcNameServiceResolveAnyIDResponse {
	// TODO: implement
	// TODO: test

	return &pb.RpcNameServiceResolveAnyIDResponse{
		Error: &pb.RpcNameServiceResolveAnyIDResponseError{
			Code:        pb.RpcNameServiceResolveAnyIDResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) NameServiceResolveSpaceID(ctx context.Context, req *pb.RpcNameServiceResolveSpaceIDRequest) *pb.RpcNameServiceResolveSpaceIDResponse {
	// TODO: implement
	// TODO: test

	return &pb.RpcNameServiceResolveSpaceIDResponse{
		Error: &pb.RpcNameServiceResolveSpaceIDResponseError{
			Code:        pb.RpcNameServiceResolveSpaceIDResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

/*
// NameServiceReverseResolveName does a reverse name lookup: address -> somename.any
func (mw *Middleware) NameServiceReverseResolveName(ctx context.Context, req *pb.RpcNameServiceReverseResolveNameRequest) *pb.RpcNameServiceReverseResolveNameResponse {
	// Get name service object that connects to the remote "namingNode"
	// in order for that to work, we need to have a "namingNode" node in the nodes section of the config
	ns, err := mw.getNameService()

	if err != nil {
		return &pb.RpcNameServiceReverseResolveNameResponse{
			Error: &pb.RpcNameServiceReverseResolveNameResponseError{
				// we don't map error codes here
				Code:        pb.RpcNameServiceReverseResolveNameResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	var in proto.NameByAddressRequest
	in.OwnerScwEthAddress = req.OwnerScwEthAddress
	in.OwnerEthAddress = req.OwnerEthAddress

	nar, err := ns.GetNameByAddress(ctx, &in)
	if err != nil {
		return &pb.RpcNameServiceReverseResolveNameResponse{
			Error: &pb.RpcNameServiceReverseResolveNameResponseError{
				// we don't map error codes here
				Code:        pb.RpcNameServiceReverseResolveNameResponseError_UNKNOWN_ERROR,
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
*/

func (mw *Middleware) NameServiceUserAccountGet(ctx context.Context, req *pb.RpcNameServiceUserAccountGetRequest) *pb.RpcNameServiceUserAccountGetResponse {
	// 1 - get name service object that connects to the remote "namingNode"
	// in order for that to work, we need to have a "namingNode" node in the nodes section of the config
	ns, err := mw.getNameService()

	if err != nil {
		return &pb.RpcNameServiceUserAccountGetResponse{
			Error: &pb.RpcNameServiceUserAccountGetResponseError{
				// we don't map error codes here
				Code:        pb.RpcNameServiceUserAccountGetResponseError_UNKNOWN_ERROR,
				Description: err.Error(),
			},
		}
	}

	// 2 - get user's ETH address from the wallet
	w, err := mw.getWallet()
	if err != nil {
		return &pb.RpcNameServiceUserAccountGetResponse{
			Error: &pb.RpcNameServiceUserAccountGetResponseError{
				Code:        pb.RpcNameServiceUserAccountGetResponseError_NOT_LOGGED_IN,
				Description: err.Error(),
			},
		}
	}

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
