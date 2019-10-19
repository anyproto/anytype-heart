package core

import (
	"os"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

const wordCount int = 12

func (mw *Middleware) WalletCreate(req *pb.WalletCreateRequest) *pb.WalletCreateResponse {
	response := func(mnemonic string, code pb.WalletCreateResponse_Error_Code, err error) *pb.WalletCreateResponse {
		m := &pb.WalletCreateResponse{Mnemonic: mnemonic, Error: &pb.WalletCreateResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	mw.rootPath = req.RootPath
	mw.localAccounts = nil

	err := os.MkdirAll(mw.rootPath, 0700)
	if err != nil {
		return response("", pb.WalletCreateResponse_Error_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	mnemonic, err := core.WalletGenerateMnemonic(wordCount)
	if err != nil {
		return response("", pb.WalletCreateResponse_Error_UNKNOWN_ERROR, err)
	}

	mw.mnemonic = mnemonic

	return response(mnemonic, pb.WalletCreateResponse_Error_NULL, nil)
}

func (mw *Middleware) WalletRecover(req *pb.WalletRecoverRequest) *pb.WalletRecoverResponse {
	response := func(code pb.WalletRecoverResponse_Error_Code, err error) *pb.WalletRecoverResponse {
		m := &pb.WalletRecoverResponse{Error: &pb.WalletRecoverResponse_Error{Code: code}}
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
		return response(pb.WalletRecoverResponse_Error_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	// test if mnemonic is correct
	_, err = core.WalletAccountAt(req.Mnemonic, 0, "")
	if err != nil {
		return response(pb.WalletRecoverResponse_Error_BAD_INPUT, err)
	}

	return response(pb.WalletRecoverResponse_Error_NULL, nil)
}
