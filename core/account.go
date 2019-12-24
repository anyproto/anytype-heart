package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) AccountCreate(req *pb.RpcAccountCreateRequest) *pb.RpcAccountCreateResponse {
	response := func(account *model.Account, code pb.RpcAccountCreateResponseErrorCode, err error) *pb.RpcAccountCreateResponse {
		m := &pb.RpcAccountCreateResponse{Account: account, Error: &pb.RpcAccountCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.accountSearchCancel != nil {
		// this func will wait until search process will stop in order to be sure node was properly stopped
		mw.accountSearchCancel()
	}

	account, err := core.WalletAccountAt(mw.mnemonic, len(mw.localAccounts), "")
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	err = core.WalletInitRepo(mw.rootPath, account.Seed())
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	anytype, err := core.New(mw.rootPath, account.Address())
	if err != nil {
		return response(nil, pb.RpcAccountCreateResponseError_UNKNOWN_ERROR, err)
	}

	mw.Anytype = anytype
	newAcc := &model.Account{Id: account.Address()}

	err = mw.Run()
	if err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_START_NODE, err)
	}

	err = mw.AccountSetName(req.Name)
	if err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	}
	newAcc.Name, err = mw.Textile.Name()
	if err != nil {
		return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_NAME, err)
	}

	if req.GetAvatarLocalPath() != "" {
		_, err := mw.AccountSetAvatar(req.GetAvatarLocalPath())
		if err != nil {
			return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR, err)
		}

		hash, err := mw.Textile.Avatar()
		if err != nil {
			return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR, err)
		}
		newAcc.Avatar = &model.AccountAvatar{Avatar: &model.AccountAvatarAvatarOfImage{Image: &model.BlockContentFile{Hash: hash}}}
	} else if req.GetAvatarColor() != "" {
		err := mw.AccountSetAvatarColor(req.GetAvatarColor())
		if err != nil {
			return response(newAcc, pb.RpcAccountCreateResponseError_ACCOUNT_CREATED_BUT_FAILED_TO_SET_AVATAR, err)
		}
	}

	mw.localAccounts = append(mw.localAccounts, newAcc)
	mw.switchAccount(newAcc.Id)
	return response(newAcc, pb.RpcAccountCreateResponseError_NULL, nil)
}

func (mw *Middleware) AccountRecover(_ *pb.RpcAccountRecoverRequest) *pb.RpcAccountRecoverResponse {
	response := func(code pb.RpcAccountRecoverResponseErrorCode, err error) *pb.RpcAccountRecoverResponse {
		m := &pb.RpcAccountRecoverResponse{Error: &pb.RpcAccountRecoverResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	sendAccountAddEvent := func(index int, account *model.Account) {
		m := &pb.Event{Messages: []*pb.EventMessage{{&pb.EventMessageValueOfAccountShow{AccountShow: &pb.EventAccountShow{Index: int32(index), Account: account}}}}}
		if mw.SendEvent != nil {
			mw.SendEvent(m)
		}
	}

	if mw.mnemonic == "" {
		return response(pb.RpcAccountRecoverResponseError_NEED_TO_RECOVER_WALLET_FIRST, nil)
	}

	shouldCancel := false
	var accountSearchFinished = make(chan struct{}, 1)
	var searchQueryCancel context.CancelFunc

	mw.accountSearchCancel = func() {
		shouldCancel = true
		if searchQueryCancel != nil {
			searchQueryCancel()
		}

		<-accountSearchFinished
	}

	defer func() {
		accountSearchFinished <- struct{}{}
	}()

	for index := 0; index < len(mw.localAccounts); index++ {
		// in case we returned to the account choose screen we can use cached accounts
		sendAccountAddEvent(index, mw.localAccounts[index])
		index++
		if shouldCancel {
			return response(pb.RpcAccountRecoverResponseError_NULL, nil)
		}
	}

	// now let's start the first account to perform cafe contacts search queries
	account, err := core.WalletAccountAt(mw.mnemonic, 0, "")
	if err != nil {
		return response(pb.RpcAccountRecoverResponseError_WALLET_RECOVER_NOT_PERFORMED, err)
	}

	err = core.WalletInitRepo(mw.rootPath, account.Seed())
	if err != nil && err != core.ErrRepoExists {
		return response(pb.RpcAccountRecoverResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	mw.Anytype, err = core.New(mw.rootPath, account.Address())
	if err != nil {
		return response(pb.RpcAccountRecoverResponseError_UNKNOWN_ERROR, err)
	}

	err = mw.Anytype.Run()
	if err != nil {
		if err == core.ErrRepoCorrupted {
			return response(pb.RpcAccountRecoverResponseError_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
		}

		return response(pb.RpcAccountRecoverResponseError_FAILED_TO_RUN_NODE, err)
	}

	if shouldCancel {
		return response(pb.RpcAccountRecoverResponseError_NULL, nil)
	}

	for {
		if mw.Anytype.Textile.Node().Online() {
			break
		}
		time.Sleep(time.Second)
	}

	for {
		// wait for cafe registration
		// in order to use cafeAPI instead of pubsub
		if cs := mw.Anytype.Textile.Node().CafeSessions(); cs != nil && len(cs.Items) > 0 {
			break
		}

		time.Sleep(time.Second)
	}

	index := 0
	for {
		// todo: add goroutine to query multiple accounts at once
		account, err := core.WalletAccountAt(mw.mnemonic, index, "")
		if err != nil {
			return response(pb.RpcAccountRecoverResponseError_WALLET_RECOVER_NOT_PERFORMED, err)
		}

		var ctx context.Context
		ctx, searchQueryCancel = context.WithCancel(context.Background())
		contact, err := mw.Anytype.AccountRequestStoredContact(ctx, account.Address())

		if err != nil || contact == nil {
			if index == 0 {
				return response(pb.RpcAccountRecoverResponseError_NO_ACCOUNTS_FOUND, err)
			}
			return response(pb.RpcAccountRecoverResponseError_NULL, nil)
		}

		if contact.Name == "" {
			if index == 0 {
				return response(pb.RpcAccountRecoverResponseError_NO_ACCOUNTS_FOUND, err)
			}

			return response(pb.RpcAccountRecoverResponseError_NULL, nil)
		}

		newAcc := &model.Account{Id: account.Address(), Name: contact.Name}

		if contact.Avatar != "" {
			newAcc.Avatar = getAvatarFromString(contact.Avatar)
		}

		sendAccountAddEvent(index, newAcc)
		mw.localAccounts = append(mw.localAccounts, newAcc)

		if shouldCancel {
			return response(pb.RpcAccountRecoverResponseError_NULL, nil)
		}
		index++
	}
}

func (mw *Middleware) AccountSelect(req *pb.RpcAccountSelectRequest) *pb.RpcAccountSelectResponse {
	response := func(account *model.Account, code pb.RpcAccountSelectResponseErrorCode, err error) *pb.RpcAccountSelectResponse {
		m := &pb.RpcAccountSelectResponse{Account: account, Error: &pb.RpcAccountSelectResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.Anytype != nil && req.Id == mw.Anytype.Textile.Address() {
		acc := &model.Account{Id: req.Id}

		var err error
		acc.Name, err = mw.Anytype.Textile.Name()
		if err != nil {
			return response(acc, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO, err)
		}

		avatarHashOrColor, err := mw.Anytype.Textile.Avatar()
		if err != nil {
			return response(acc, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO, err)
		}

		acc.Avatar = getAvatarFromString(avatarHashOrColor)
		return response(acc, pb.RpcAccountSelectResponseError_NULL, nil)
	} else if mw.Anytype != nil {
		// user chose account other than the first one
		// we need to stop the first node that what used to search other accounts and then start the right one
		err := mw.Stop()
		if err != nil {
			return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_STOP_SEARCHER_NODE, err)
		}
	}

	if req.RootPath != "" {
		mw.rootPath = req.RootPath
	}

	if mw.accountSearchCancel != nil {
		// this func will wait until search process will stop in order to be sure node was properly stopped
		mw.accountSearchCancel()
	}

	if _, err := os.Stat(filepath.Join(mw.rootPath, req.Id)); os.IsNotExist(err) {
		if mw.mnemonic == "" {
			return response(nil, pb.RpcAccountSelectResponseError_LOCAL_REPO_NOT_EXISTS_AND_MNEMONIC_NOT_SET, err)
		}

		account, err := core.WalletAccountAt(mw.mnemonic, len(mw.localAccounts), "")
		if err != nil {
			return response(nil, pb.RpcAccountSelectResponseError_UNKNOWN_ERROR, err)
		}

		err = core.WalletInitRepo(mw.rootPath, account.Seed())
		if err != nil {
			return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_CREATE_LOCAL_REPO, err)
		}
	}

	anytype, err := core.New(mw.rootPath, req.Id)
	if err != nil {
		return response(nil, pb.RpcAccountSelectResponseError_UNKNOWN_ERROR, err)
	}

	mw.Anytype = anytype

	err = mw.Run()
	if err != nil {
		if err == core.ErrRepoCorrupted {
			return response(nil, pb.RpcAccountSelectResponseError_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
		}

		return response(nil, pb.RpcAccountSelectResponseError_FAILED_TO_RUN_NODE, err)
	}

	acc := &model.Account{Id: req.Id}

	acc.Name, err = mw.Anytype.Textile.Name()
	if err != nil {
		return response(acc, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO, err)
	}

	avatarHashOrColor, err := mw.Anytype.Textile.Avatar()
	if err != nil {
		return response(acc, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO, err)
	}

	if acc.Name == "" && avatarHashOrColor == "" {
		for {
			// wait for cafe registration
			// in order to use cafeAPI instead of pubsub
			if cs := anytype.Textile.Node().CafeSessions(); cs != nil && len(cs.Items) > 0 {
				break
			}

			time.Sleep(time.Second)
		}

		contact, err := anytype.AccountRequestStoredContact(context.Background(), req.Id)
		if err != nil {
			return response(acc, pb.RpcAccountSelectResponseError_FAILED_TO_FIND_ACCOUNT_INFO, err)
		}
		acc.Name = contact.Name
		avatarHashOrColor = contact.Avatar
	}

	if avatarHashOrColor != "" {
		acc.Avatar = getAvatarFromString(avatarHashOrColor)
	}
	mw.switchAccount(acc.Id)
	return response(acc, pb.RpcAccountSelectResponseError_NULL, nil)
}

func getAvatarFromString(avatarHashOrColor string) *model.AccountAvatar {
	if strings.HasPrefix(avatarHashOrColor, "#") {
		return &model.AccountAvatar{Avatar: &model.AccountAvatarAvatarOfColor{avatarHashOrColor}}
	} else {
		return &model.AccountAvatar{
			&model.AccountAvatarAvatarOfImage{&model.BlockContentFile{Hash: avatarHashOrColor}},
		}
	}
}
