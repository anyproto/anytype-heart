package core

import (
	"context"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/core/domain"
)

func (mw *Middleware) WalletCreate(cctx context.Context, req *pb.RpcWalletCreateRequest) *pb.RpcWalletCreateResponse {
	response := func(mnemonic string, code pb.RpcWalletCreateResponseErrorCode, err error) *pb.RpcWalletCreateResponse {
		m := &pb.RpcWalletCreateResponse{Mnemonic: mnemonic, Error: &pb.RpcWalletCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	mnemonic, err := mw.applicationService.WalletCreate(req)
	code, err := domain.UnwrapCodeFromError[pb.RpcWalletCreateResponseErrorCode](err)
	return response(mnemonic, code, err)
}

func (mw *Middleware) WalletRecover(cctx context.Context, req *pb.RpcWalletRecoverRequest) *pb.RpcWalletRecoverResponse {
	response := func(code pb.RpcWalletRecoverResponseErrorCode, err error) *pb.RpcWalletRecoverResponse {
		m := &pb.RpcWalletRecoverResponse{Error: &pb.RpcWalletRecoverResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.applicationService.WalletRecover(req)
	code, err := domain.UnwrapCodeFromError[pb.RpcWalletRecoverResponseErrorCode](err)
	return response(code, err)
}

func (mw *Middleware) WalletConvert(cctx context.Context, req *pb.RpcWalletConvertRequest) *pb.RpcWalletConvertResponse {
	response := func(mnemonic, entropy string, code pb.RpcWalletConvertResponseErrorCode, err error) *pb.RpcWalletConvertResponse {
		m := &pb.RpcWalletConvertResponse{Mnemonic: mnemonic, Entropy: entropy, Error: &pb.RpcWalletConvertResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	mnemonic, entropy, err := mw.applicationService.WalletConvert(req)
	code, err := domain.UnwrapCodeFromError[pb.RpcWalletConvertResponseErrorCode](err)
	return response(mnemonic, entropy, code, err)
}

func (mw *Middleware) WalletCreateSession(cctx context.Context, req *pb.RpcWalletCreateSessionRequest) *pb.RpcWalletCreateSessionResponse {
	response := func(token string, code pb.RpcWalletCreateSessionResponseErrorCode, err error) *pb.RpcWalletCreateSessionResponse {
		m := &pb.RpcWalletCreateSessionResponse{Token: token, Error: &pb.RpcWalletCreateSessionResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	token, err := mw.applicationService.CreateSession(req)
	code, err := domain.UnwrapCodeFromError[pb.RpcWalletCreateSessionResponseErrorCode](err)
	return response(token, code, err)
}

func (mw *Middleware) WalletCloseSession(cctx context.Context, req *pb.RpcWalletCloseSessionRequest) *pb.RpcWalletCloseSessionResponse {
	response := func(code pb.RpcWalletCloseSessionResponseErrorCode, err error) *pb.RpcWalletCloseSessionResponse {
		m := &pb.RpcWalletCloseSessionResponse{Error: &pb.RpcWalletCloseSessionResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	err := mw.applicationService.CloseSession(req)
	code, err := domain.UnwrapCodeFromError[pb.RpcWalletCloseSessionResponseErrorCode](err)
	return response(code, err)
}
