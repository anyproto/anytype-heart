package core

import (
	"os"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

const wordCount int = 12

func (mw *Middleware) WalletCreate(req *pb.Rpc_Wallet_Create_Request) *pb.Rpc_Wallet_Create_Response {
	response := func(mnemonic string, code pb.Rpc_Wallet_Create_Response_Error_Code, err error) *pb.Rpc_Wallet_Create_Response {
		m := &pb.Rpc_Wallet_Create_Response{Mnemonic: mnemonic, Error: &pb.Rpc_Wallet_Create_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	mw.rootPath = req.RootPath
	mw.localAccounts = nil

	err := os.MkdirAll(mw.rootPath, 0700)
	if err != nil {
		return response("", pb.Rpc_Wallet_Create_Response_Error_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	mnemonic, err := core.WalletGenerateMnemonic(wordCount)
	if err != nil {
		return response("", pb.Rpc_Wallet_Create_Response_Error_UNKNOWN_ERROR, err)
	}

	mw.mnemonic = mnemonic

	return response(mnemonic, pb.Rpc_Wallet_Create_Response_Error_NULL, nil)
}

func (mw *Middleware) WalletRecover(req *pb.Rpc_Wallet_Recover_Request) *pb.Rpc_Wallet_Recover_Response {
	response := func(code pb.Rpc_Wallet_Recover_Response_Error_Code, err error) *pb.Rpc_Wallet_Recover_Response {
		m := &pb.Rpc_Wallet_Recover_Response{Error: &pb.Rpc_Wallet_Recover_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.mnemonic != req.Mnemonic {
		mw.localAccounts = nil
	}

	mw.mnemonic = req.Mnemonic
	mw.rootPath = req.RootPath

	err := os.MkdirAll(mw.rootPath, 0700)
	if err != nil {
		return response(pb.Rpc_Wallet_Recover_Response_Error_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	// test if mnemonic is correct
	_, err = core.WalletAccountAt(req.Mnemonic, 0, "")
	if err != nil {
		return response(pb.Rpc_Wallet_Recover_Response_Error_BAD_INPUT, err)
	}

	return response(pb.Rpc_Wallet_Recover_Response_Error_NULL, nil)
}
