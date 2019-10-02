package lib

import (
	"context"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
)

var avatarSizes = []pb.ImageSize{pb.ImageSize_SMALL, pb.ImageSize_LARGE}

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

	if mw.accountSearchCancel != nil {
		// this func will wait until search process will stop in order to be sure node was properly stopped
		mw.accountSearchCancel()
	}

	account, err := core.WalletAccountAt(mw.mnemonic, len(mw.localAccounts), "")
	if err != nil {
		return response(nil, pb.AccountCreateResponse_Error_UNKNOWN_ERROR, err)
	}

	err = core.WalletInitRepo(mw.rootPath, account.Seed())
	if err != nil {
		return response(nil, pb.AccountCreateResponse_Error_UNKNOWN_ERROR, err)
	}

	anytype, err := core.New(mw.rootPath, account.Address())
	if err != nil {
		return response(nil, pb.AccountCreateResponse_Error_UNKNOWN_ERROR, err)
	}

	mw.Anytype = anytype
	newAcc := &pb.Account{Id: account.Address()}

	err = mw.Run()
	if err != nil {
		return response(newAcc, pb.AccountCreateResponse_Error_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE, err)
	}

	err = mw.AccountSetName(q.Username)
	if err != nil {
		return response(newAcc, pb.AccountCreateResponse_Error_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	}
	newAcc.Name, err = mw.Textile.Name()
	if err != nil {
		return response(newAcc, pb.AccountCreateResponse_Error_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	}

	if q.AvatarLocalPath != "" {
		_, err := mw.AccountSetAvatar(q.AvatarLocalPath)
		if err != nil {
			return response(newAcc, pb.AccountCreateResponse_Error_ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR, err)
		}

		hash, err := mw.Textile.Avatar()
		if err != nil {
			return response(newAcc, pb.AccountCreateResponse_Error_ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR, err)
		}

		newAcc.Avatar = &pb.Image{Id: hash, Sizes: avatarSizes}
	}

	mw.localAccounts = append(mw.localAccounts, newAcc)
	return response(newAcc, pb.AccountCreateResponse_Error_NULL, nil)
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

	// Currently it is possible to choose not existing index – this will create the new account
	// todo: decide if this is ok

	if mw.accountSearchCancel != nil {
		// this func will wait until search process will stop in order to be sure node was properly stopped
		mw.accountSearchCancel()
	}

	anytype, err := core.New(mw.rootPath, q.Id)
	if err != nil {
		return response(nil, pb.AccountSelectResponse_Error_UNKNOWN_ERROR, err)
	}

	mw.Anytype = anytype

	err = mw.Run()
	if err != nil {
		if err == core.ErrRepoCorrupted {
			return response(nil, pb.AccountSelectResponse_Error_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
		}

		return response(nil, pb.AccountSelectResponse_Error_FAILED_TO_RUN_NODE, err)
	}

	acc := &pb.Account{Id: q.Id}

	acc.Name, err = mw.Anytype.Textile.Name()
	if err != nil {
		return response(acc, pb.AccountSelectResponse_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
	}

	avatarHash, err := mw.Anytype.Textile.Avatar()
	if err != nil {
		return response(acc, pb.AccountSelectResponse_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
	}

	if acc.Name == "" && avatarHash == "" {
		for {
			// wait for cafe registration
			// in order to use cafeAPI instead of pubsub
			if cs := anytype.Textile.Node().CafeSessions(); cs != nil && len(cs.Items) > 0 {
				break
			}

			time.Sleep(time.Second)
		}

		contact, err := anytype.AccountRequestStoredContact(context.Background(), q.Id)
		if err != nil {
			return response(acc, pb.AccountSelectResponse_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
		}
		acc.Name = contact.Name
		avatarHash = contact.Avatar
	}

	if avatarHash != "" {
		acc.Avatar = &pb.Image{Id: avatarHash, Sizes: avatarSizes}
	}

	return response(acc, pb.AccountSelectResponse_Error_NULL, nil)
}
