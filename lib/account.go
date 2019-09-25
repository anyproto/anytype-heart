package main

import (
	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
)

//exportMobile AccountCreate
func AccountCreate(b []byte) []byte {
	response := func(account *pb.Account, code pb.AccountCreateResponse_Error_Code, err error) []byte {
		m := &pb.AccountCreateResponse{Account: account, Error: &pb.AccountCreateResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return Marshal(m)
	}

	var q pb.AccountCreateRequest
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(nil, pb.AccountCreateResponse_Error_BAD_INPUT, err)
	}

	// this func will wait until search process will stop in order to be sure node was properly stopped
	instance.accountSearchCancel()

	account, err := core.WalletAccountAt(instance.mnemonic, len(instance.localAccounts), "")
	if err != nil {
		return response(nil, pb.AccountCreateResponse_Error_UNKNOWN_ERROR, err)
	}

	err = core.WalletInitRepo(instance.rootPath, account.Seed())
	if err != nil {
		return response(nil, pb.AccountCreateResponse_Error_UNKNOWN_ERROR, err)
	}

	anytype, err := core.New(instance.rootPath, account.Address())
	if err != nil {
		return response(nil, pb.AccountCreateResponse_Error_UNKNOWN_ERROR, err)
	}

	instance.Anytype = anytype
	newAcc := &pb.Account{Id: account.Address()}

	err = instance.Run()
	if err != nil {
		return response(newAcc, pb.AccountCreateResponse_Error_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE, err)
	}

	err = instance.AccountSetName(q.Username)
	if err != nil {
		return response(newAcc, pb.AccountCreateResponse_Error_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	}

	if q.AvatarLocalPath != "" {
		hash, err := instance.AccountSetAvatar(q.AvatarLocalPath)
		if err != nil {
			return response(newAcc, pb.AccountCreateResponse_Error_ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR, err)
		}
		newAcc.Avatar = ipfsFileURL(hash, q.AvatarLocalPath)
	}

	instance.localAccounts = append(instance.localAccounts, newAcc)
	return response(newAcc, pb.AccountCreateResponse_Error_ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR, err)
}

//exportMobile AccountSelect
func AccountSelect(b []byte) []byte {
	response := func(account *pb.Account, code pb.AccountSelectResponse_Error_Code, err error) []byte {
		m := &pb.AccountSelectResponse{Account: account, Error: &pb.AccountSelectResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return Marshal(m)
	}

	var q pb.AccountSelectRequest
	err := proto.Unmarshal(b, &q)

	if err != nil {
		return response(nil, pb.AccountSelectResponse_Error_BAD_INPUT, err)
	}
	account, err := core.WalletAccountAt(instance.mnemonic, int(q.Index), "")
	if err != nil {
		return response(nil, pb.AccountSelectResponse_Error_BAD_INPUT, err)
	}

	// this func will wait until search process will stop in order to be sure node was properly stopped
	instance.accountSearchCancel()

	anytype, err := core.New(instance.rootPath, account.Address())
	if err != nil {
		return response(nil, pb.AccountSelectResponse_Error_UNKNOWN_ERROR, err)
	}

	instance.Anytype = anytype

	err = instance.Run()
	if err != nil {
		if err == core.ErrRepoCorrupted {
			return response(nil, pb.AccountSelectResponse_Error_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
		}

		return response(nil, pb.AccountSelectResponse_Error_FAILED_TO_RUN_NODE, err)
	}

	acc := &pb.Account{Id: account.Address()}

	acc.Name, err = instance.Anytype.Textile.Name()
	if err != nil {
		return response(acc, pb.AccountSelectResponse_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
	}

	acc.Avatar, err = instance.Anytype.Textile.Avatar()
	if err != nil {
		return response(acc, pb.AccountSelectResponse_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
	}

	return response(acc, pb.AccountSelectResponse_Error_NULL, nil)
}
