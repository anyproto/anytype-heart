package nameservice

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nameservice/nameserviceclient"

	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"

	proto "github.com/anyproto/any-sync/nameservice/nameserviceproto"
)

const CName = "nameservice"

var log = logging.Logger(CName).Desugar()

var (
	ErrBadResolve = errors.New("can not resolve anyname")
)

func NsNameToFullName(nsName string, nsNameType model.NameserviceNameType) string {
	// if no name - return empty string
	if nsName == "" {
		return ""
	}

	if nsNameType == model.NameserviceNameType_AnyName {
		return nsName + ".any"
	}

	// by default return it
	return nsName
}

type Service interface {
	NameServiceResolveName(ctx context.Context, req *pb.RpcNameServiceResolveNameRequest) (*pb.RpcNameServiceResolveNameResponse, error)
	NameServiceResolveAnyId(ctx context.Context, req *pb.RpcNameServiceResolveAnyIdRequest) (*pb.RpcNameServiceResolveAnyIdResponse, error)
	NameServiceUserAccountGet(ctx context.Context, req *pb.RpcNameServiceUserAccountGetRequest) (*pb.RpcNameServiceUserAccountGetResponse, error)

	app.Component
}

func New() Service {
	return &service{}
}

type service struct {
	nsclient nameserviceclient.AnyNsClientService
	wallet   wallet.Wallet
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Init(a *app.App) (err error) {
	// Get name service object that connects to the remote "namingNode"
	// in order for that to work, we need to have a "namingNode" node in the nodes section of the config
	s.nsclient = app.MustComponent[nameserviceclient.AnyNsClientService](a)
	s.wallet = app.MustComponent[wallet.Wallet](a)
	return nil
}

func (s *service) NameServiceResolveName(ctx context.Context, req *pb.RpcNameServiceResolveNameRequest) (*pb.RpcNameServiceResolveNameResponse, error) {
	var in proto.NameAvailableRequest
	in.FullName = NsNameToFullName(req.NsName, req.NsNameType)

	nar, err := s.nsclient.IsNameAvailable(ctx, &in)
	if err != nil {
		return nil, err
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

	return &out, nil
}

func FullNameToNsName(fullName string) (nsName string, nsNameType model.NameserviceNameType) {
	// if no name - return empty string
	if fullName == "" {
		return "", model.NameserviceNameType_AnyName
	}

	// remove .any from the name
	if fullName[len(fullName)-4:] == ".any" {
		return fullName[:len(fullName)-4], model.NameserviceNameType_AnyName
	}

	// by default return it
	return fullName, model.NameserviceNameType_AnyName
}

func (s *service) NameServiceResolveAnyId(ctx context.Context, req *pb.RpcNameServiceResolveAnyIdRequest) (*pb.RpcNameServiceResolveAnyIdResponse, error) {
	var in proto.NameByAnyIdRequest
	in.AnyAddress = req.AnyId

	nar, err := s.nsclient.GetNameByAnyId(ctx, &in)
	if err != nil {
		return nil, err
	}

	// Return the response
	var out pb.RpcNameServiceResolveAnyIdResponse
	out.Found = nar.Found
	out.NsName, out.NsNameType = FullNameToNsName(nar.Name)

	return &out, nil
}

func (s *service) NameServiceUserAccountGet(ctx context.Context, req *pb.RpcNameServiceUserAccountGetRequest) (*pb.RpcNameServiceUserAccountGetResponse, error) {
	// when AccountAbstraction is used to deploy a smart contract wallet
	// then name is really owned by this SCW, but owner of this SCW is
	// EOA that was used to sign transaction
	//
	// EOA (w.GetAccountEthAddress()) -> SCW (ua.OwnerSmartContracWalletAddress) -> name
	var guar proto.GetUserAccountRequest
	guar.OwnerEthAddress = s.wallet.GetAccountEthAddress().Hex()

	ua, err := s.nsclient.GetUserAccount(ctx, &guar)
	if err != nil {
		return nil, err
	}

	// 4 - check if any name is attached to the account (reverse resolve the name)
	var in proto.NameByAddressRequest

	// NOTE: we are passing here SCW address, not initial ETH address!
	// read comment about SCW above please
	in.OwnerScwEthAddress = ua.OwnerSmartContracWalletAddress

	nar, err := s.nsclient.GetNameByAddress(ctx, &in)
	if err != nil {
		return nil, ErrBadResolve
	}

	// Return the response
	var out pb.RpcNameServiceUserAccountGetResponse
	out.NamesCountLeft = ua.NamesCountLeft
	out.OperationsCountLeft = ua.OperationsCountLeft
	// not checking nar.Found here, no need
	out.NsNameAttached, out.NsNameType = FullNameToNsName(nar.Name)

	return &out, nil
}
