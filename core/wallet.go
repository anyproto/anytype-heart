package core

import (
	"os"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

const wordCount int = 12

func (mw *Middleware) WalletCreate(req *pb.RpcWalletCreateRequest) *pb.RpcWalletCreateResponse {
	response := func(mnemonic string, code pb.RpcWalletCreateResponseErrorCode, err error) *pb.RpcWalletCreateResponse {
		m := &pb.RpcWalletCreateResponse{Mnemonic: mnemonic, Error: &pb.RpcWalletCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	mw.m.Lock()
	defer mw.m.Unlock()

	mw.rootPath = req.RootPath
	mw.foundAccounts = nil

	err := os.MkdirAll(mw.rootPath, 0700)
	if err != nil {
		return response("", pb.RpcWalletCreateResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	mnemonic, err := core.WalletGenerateMnemonic(wordCount)
	if err != nil {
		return response("", pb.RpcWalletCreateResponseError_UNKNOWN_ERROR, err)
	}

	mw.mnemonic = mnemonic

	return response(mnemonic, pb.RpcWalletCreateResponseError_NULL, nil)
}

func (mw *Middleware) WalletRecover(req *pb.RpcWalletRecoverRequest) *pb.RpcWalletRecoverResponse {
	response := func(code pb.RpcWalletRecoverResponseErrorCode, err error) *pb.RpcWalletRecoverResponse {
		m := &pb.RpcWalletRecoverResponse{Error: &pb.RpcWalletRecoverResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	mw.accountSearchCancelAndWait()

	mw.m.Lock()
	defer mw.m.Unlock()

	if mw.mnemonic == req.Mnemonic {
		return response(pb.RpcWalletRecoverResponseError_NULL, nil)
	}

	mw.mnemonic = req.Mnemonic
	mw.rootPath = req.RootPath

	err := os.MkdirAll(mw.rootPath, 0700)
	if err != nil {
		return response(pb.RpcWalletRecoverResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	// test if mnemonic is correct
	_, err = core.WalletAccountAt(req.Mnemonic, 0, "")
	if err != nil {
		return response(pb.RpcWalletRecoverResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcWalletRecoverResponseError_NULL, nil)
}
