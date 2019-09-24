package main

import (
	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
)

//exportMobile AccountCreate
func AccountCreate(b []byte) []byte {
	response := func(account *pb.Account, code pb.AccountCreateR_Error_Code, err error) []byte{
		m := &pb.AccountCreateR{Account: account, Error: &pb.AccountCreateR_Error{Code: code}}
		if err != nil {
			m.Error.Desc = err.Error()
		}

		return Marshal(m)
	}

	var q pb.AccountCreateQ
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(nil, pb.AccountCreateR_Error_BAD_INPUT, err)
	}

	account, err := core.WalletAccountAt(instance.mnemonic, 0, "")
	if err != nil {
		return response(nil, pb.AccountCreateR_Error_UNKNOWN_ERROR, err)
	}

	err = core.WalletInitRepo(instance.rootPath, account.Seed())
	if err != nil {
		return response(nil, pb.AccountCreateR_Error_UNKNOWN_ERROR, err)
	}

	anytype, err := core.New(account.Address())
	if err != nil {
		return response(nil, pb.AccountCreateR_Error_UNKNOWN_ERROR, err)
	}

	instance.Anytype = anytype

	err = instance.Run()
	if err != nil {
		return response(nil, pb.AccountCreateR_Error_FAILED_TO_START_NODE, err)
	}

	err = instance.AccountSetName(q.Username)
	if err != nil {
		return response(nil, pb.AccountCreateR_Error_FAILED_TO_SET_NAME, err)
	}

	_, err = instance.AccountSetAvatar(q.AvatarLocalPath)
	if err != nil {
		return response(nil, pb.AccountCreateR_Error_FAILED_TO_SET_AVATAR, err)
	}

	return response(&pb.Account{Id: account.Address(), Name: q.Username, Avatar: q.AvatarLocalPath}, pb.AccountCreateR_Error_FAILED_TO_SET_AVATAR, err)
}


//exportMobile AccountSelect
func AccountSelect(b []byte) []byte {
	response := func(account *pb.Account, code pb.AccountSelectR_Error_Code, err error) []byte{
		m := &pb.AccountSelectR{Account: account, Error: &pb.AccountSelectR_Error{Code: code}}
		if err != nil {
			m.Error.Desc = err.Error()
		}

		return Marshal(m)
	}

	var q pb.AccountSelectQ
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(nil, pb.AccountSelectR_Error_BAD_INPUT, err)
	}
	account, err := core.WalletAccountAt(instance.mnemonic, int(q.Index), "")
	if err != nil {
		return response(nil, pb.AccountSelectR_Error_BAD_INPUT, err)
	}

	anytype, err := core.New(account.Address())
	if err != nil {
		return response(nil, pb.AccountSelectR_Error_UNKNOWN_ERROR, err)
	}

	instance.Anytype = anytype

	err = instance.Run()
	if err != nil {
		if err == core.ErrRepoCorrupted {
			return response(nil, pb.AccountSelectR_Error_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
		}

		return response(nil, pb.AccountSelectR_Error_FAILED_TO_RUN_NODE, err)
	}

	name, err := instance.Anytype.Textile.Name()
	if err != nil {
		return response(nil, pb.AccountSelectR_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
	}

	avatar, err := instance.Anytype.Textile.Avatar()
	if err != nil {
		return response(nil, pb.AccountSelectR_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
	}

	return response(&pb.Account{Id: account.Address(), Name: name, Avatar: avatar}, pb.AccountSelectR_Error_NULL, nil)
}
