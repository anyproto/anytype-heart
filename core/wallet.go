package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/application"
	"github.com/anyproto/anytype-heart/core/wallet"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) WalletCreate(cctx context.Context, req *pb.RpcWalletCreateRequest) *pb.RpcWalletCreateResponse {
	mnemonic, accountKey, err := mw.applicationService.WalletCreate(req)
	code := mapErrorCode(err,
		errToCode(application.ErrFailedToCreateLocalRepo, pb.RpcWalletCreateResponseError_FAILED_TO_CREATE_LOCAL_REPO),
	)
	return &pb.RpcWalletCreateResponse{
		Mnemonic:   mnemonic,
		AccountKey: accountKey,
		Error: &pb.RpcWalletCreateResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) WalletRecover(cctx context.Context, req *pb.RpcWalletRecoverRequest) *pb.RpcWalletRecoverResponse {
	err := mw.applicationService.WalletRecover(req)
	code := mapErrorCode(err,
		errToCode(application.ErrBadInput, pb.RpcWalletRecoverResponseError_BAD_INPUT),
		errToCode(application.ErrFailedToCreateLocalRepo, pb.RpcWalletRecoverResponseError_FAILED_TO_CREATE_LOCAL_REPO),
	)
	return &pb.RpcWalletRecoverResponse{
		Error: &pb.RpcWalletRecoverResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) WalletConvert(cctx context.Context, req *pb.RpcWalletConvertRequest) *pb.RpcWalletConvertResponse {
	mnemonic, entropy, err := mw.applicationService.WalletConvert(req)
	code := mapErrorCode(err,
		errToCode(application.ErrBadInput, pb.RpcWalletConvertResponseError_BAD_INPUT),
	)
	return &pb.RpcWalletConvertResponse{
		Mnemonic: mnemonic,
		Entropy:  entropy,
		Error: &pb.RpcWalletConvertResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) WalletCreateSession(cctx context.Context, req *pb.RpcWalletCreateSessionRequest) *pb.RpcWalletCreateSessionResponse {
	token, accountId, err := mw.applicationService.CreateSession(req)
	code := mapErrorCode(err,
		errToCode(application.ErrBadInput, pb.RpcWalletCreateSessionResponseError_BAD_INPUT),
		errToCode(wallet.ErrAppLinkNotFound, pb.RpcWalletCreateSessionResponseError_APP_TOKEN_NOT_FOUND_IN_THE_CURRENT_ACCOUNT),
		errToCode(application.ErrApplicationIsNotRunning, pb.RpcWalletCreateSessionResponseError_UNKNOWN_ERROR),
	)
	return &pb.RpcWalletCreateSessionResponse{
		Token:     token,
		AccountId: accountId,
		Error: &pb.RpcWalletCreateSessionResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) WalletCloseSession(cctx context.Context, req *pb.RpcWalletCloseSessionRequest) *pb.RpcWalletCloseSessionResponse {
	err := mw.applicationService.CloseSession(req)
	code := mapErrorCode[pb.RpcWalletCloseSessionResponseErrorCode](err)
	return &pb.RpcWalletCloseSessionResponse{
		Error: &pb.RpcWalletCloseSessionResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}
