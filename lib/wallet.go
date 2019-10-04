package lib

import (
	"os"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
)

const wordCount int = 12

func WalletCreate(b []byte) []byte {
	response := func(mnemonic string, code pb.WalletCreateResponse_Error_Code, err error) []byte {
		m := &pb.WalletCreateResponse{Mnemonic: mnemonic, Error: &pb.WalletCreateResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return Marshal(m)
	}

	var q pb.WalletCreateRequest
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response("", pb.WalletCreateResponse_Error_BAD_INPUT, err)
	}

	mw.rootPath = q.RootPath
	mw.localAccounts = nil

	err = os.MkdirAll(mw.rootPath, 0700)
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

func WalletRecover(b []byte) []byte {
	response := func(code pb.WalletRecoverResponse_Error_Code, err error) []byte {
		m := &pb.WalletRecoverResponse{Error: &pb.WalletRecoverResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return Marshal(m)
	}

	var q pb.WalletRecoverRequest
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(pb.WalletRecoverResponse_Error_BAD_INPUT, err)
	}

	if mw.mnemonic != q.Mnemonic {
		mw.localAccounts = nil
	}

	mw.mnemonic = q.Mnemonic
	mw.rootPath = q.RootPath

	err = os.MkdirAll(mw.rootPath, 0700)
	if err != nil {
		return response(pb.WalletRecoverResponse_Error_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	// test if mnemonic is correct
	_, err = core.WalletAccountAt(q.Mnemonic, 0, "")
	if err != nil {
		return response(pb.WalletRecoverResponse_Error_BAD_INPUT, err)
	}

	return response(pb.WalletRecoverResponse_Error_NULL, nil)
}
