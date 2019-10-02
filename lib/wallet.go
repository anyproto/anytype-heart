package lib

import (
	"context"
	"os"
	"time"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	tpb "github.com/textileio/go-textile/pb"
)

const wordCount int = 12

//exportMobile WalletCreate
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

//exportMobile WalletRecover
func WalletRecover(b []byte) []byte {
	response := func(code pb.WalletRecoverResponse_Error_Code, err error) []byte {
		m := &pb.WalletRecoverResponse{Error: &pb.WalletRecoverResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return Marshal(m)
	}

	sendAccountAddEvent := func(index int, account *pb.Account, code pb.AccountAdd_Error_Code, err error) {
		pbErr := &pb.AccountAdd_Error{Code: code}
		m := &pb.Event{Message: &pb.Event_AccountAdd{AccountAdd: &pb.AccountAdd{Index: int64(index), Account: account, Error: pbErr}}}
		if err != nil {
			pbErr.Description = err.Error()
		}

		SendEvent(m)
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

	stopNode := func(anytype *core.Anytype) {
		err = anytype.Textile.Node().Stop()
		if err != nil {
			sendAccountAddEvent(0, nil, pb.AccountAdd_Error_UNKNOWN_ERROR, err)
		}
	}

	go func() {
		defer func() {
			accountSearchFinished <- struct{}{}
		}()

		for index := 0; index < len(mw.localAccounts); index++ {
			// in case we returned to the account choose screen we can use cached accounts
			sendAccountAddEvent(index, mw.localAccounts[index], pb.AccountAdd_Error_NULL, nil)
			index++
			if shouldCancel {
				return
			}
		}

		// now let's start the first account to perform cafe contacts search queries
		account, err := core.WalletAccountAt(q.Mnemonic, 0, "")
		if err != nil {
			sendAccountAddEvent(0, nil, pb.AccountAdd_Error_BAD_INPUT, err)
			return
		}

		// todo: find a better way to brut force deterministic accounts, e.g. via cafe
		err = core.WalletInitRepo(mw.rootPath, account.Seed())
		if err != nil && err != core.ErrRepoExists {
			sendAccountAddEvent(0, nil, pb.AccountAdd_Error_FAILED_TO_CREATE_LOCAL_REPO, err)
			return
		}

		anytype, err := core.New(mw.rootPath, account.Address())
		if err != nil {
			sendAccountAddEvent(0, nil, pb.AccountAdd_Error_UNKNOWN_ERROR, err)
			return
		}
		err = anytype.Run()
		if err != nil {
			if err == core.ErrRepoCorrupted {
				sendAccountAddEvent(0, nil, pb.AccountAdd_Error_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
			}

			sendAccountAddEvent(0, nil, pb.AccountAdd_Error_FAILED_TO_RUN_NODE, err)
			return
		}

		if shouldCancel {
			stopNode(anytype)
			return
		}

		for {
			if anytype.Textile.Node().Online() {
				break
			}
			time.Sleep(time.Second)
		}

		for {
			// wait for cafe registration
			// in order to use cafeAPI instead of pubsub
			if cs := anytype.Textile.Node().CafeSessions(); cs != nil && len(cs.Items) > 0 {
				break
			}

			time.Sleep(time.Second)
		}

		index := 0
		for {
			account, err := core.WalletAccountAt(q.Mnemonic, index, "")
			if err != nil {
				sendAccountAddEvent(0, nil, pb.AccountAdd_Error_BAD_INPUT, err)
				return
			}

			var contact *tpb.Contact
			var ctx context.Context
			ctx, searchQueryCancel = context.WithCancel(context.Background())
			contact, err = anytype.AccountRequestStoredContact(ctx, account.Address())

			if err != nil || contact == nil {
				sendAccountAddEvent(index, nil, pb.AccountAdd_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
				stopNode(anytype)
				return
			}

			if contact.Name == "" {
				sendAccountAddEvent(index, nil, pb.AccountAdd_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
				stopNode(anytype)
				return
			}

			newAcc := &pb.Account{Id: account.Address(), Name: contact.Name}

			if contact.Avatar != "" {
				newAcc.Avatar = &pb.Image{Id: contact.Avatar, Sizes: avatarSizes}
			}

			sendAccountAddEvent(index, newAcc, pb.AccountAdd_Error_NULL, err)
			mw.localAccounts = append(mw.localAccounts, newAcc)

			if shouldCancel {
				stopNode(anytype)
				return
			}
			index++
		}
	}()

	return response(pb.WalletRecoverResponse_Error_NULL, nil)
}
