package main

import (
	"os"

	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
)

const wordCount int = 12

//todo: move some logic to the backend lib?
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

	instance.rootPath = q.RootPath

	err = os.MkdirAll(instance.rootPath, 0644)
	if err != nil {
		return response("", pb.WalletCreateResponse_Error_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	mnemonic, err := core.WalletGenerateMnemonic(wordCount)
	if err != nil {
		return response("", pb.WalletCreateResponse_Error_UNKNOWN_ERROR, err)
	}

	instance.mnemonic = mnemonic

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

	if instance.mnemonic != q.Mnemonic {
		instance.localAccounts = nil
	}

	instance.mnemonic = q.Mnemonic
	instance.rootPath = q.RootPath

	err = os.MkdirAll(instance.rootPath, 0644)
	if err != nil {
		return response(pb.WalletRecoverResponse_Error_FAILED_TO_CREATE_LOCAL_REPO, err)
	}

	// test if mnemonic is correct
	_, err = core.WalletAccountAt(q.Mnemonic, 0, "")
	if err != nil {
		return response(pb.WalletRecoverResponse_Error_BAD_INPUT, err)
	}

	shouldCancel := false
	var accountStoppedChan = make(chan struct{}, 1)
	instance.accountSearchCancel = func() {
		shouldCancel = true
		<-accountStoppedChan
	}

	stopNode := func(anytype *core.Anytype) {
		err = anytype.Textile.Node().Stop()
		if err != nil {
			sendAccountAddEvent(0, nil, pb.AccountAdd_Error_UNKNOWN_ERROR, err)
			accountStoppedChan <- struct{}{}
		}
	}

	go func() {
		index := 0
		for {
			// in case we returned to the account choose screen we can use cached accounts
			if len(instance.localAccounts) >= (index + 1) {
				sendAccountAddEvent(index, instance.localAccounts[index], pb.AccountAdd_Error_NULL, nil)
				index++
				if shouldCancel {
					return
				}

				continue
			}

			account, err := core.WalletAccountAt(q.Mnemonic, index, "")
			if err != nil {
				sendAccountAddEvent(index, nil, pb.AccountAdd_Error_BAD_INPUT, err)
				break
			}

			// todo: find a better way to brut force deterministic accounts, e.g. via cafe
			err = core.WalletInitRepo(instance.rootPath, account.Seed())
			if err != nil && err != core.ErrRepoExists {
				sendAccountAddEvent(index, nil, pb.AccountAdd_Error_FAILED_TO_CREATE_LOCAL_REPO, err)
				break
			}

			anytype, err := core.New(instance.rootPath, account.Address())
			if err != nil {
				sendAccountAddEvent(index, nil, pb.AccountAdd_Error_UNKNOWN_ERROR, err)
				break
			}

			err = instance.Run()
			if err != nil {
				if err == core.ErrRepoCorrupted {
					sendAccountAddEvent(index, nil, pb.AccountAdd_Error_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
				}

				sendAccountAddEvent(index, nil, pb.AccountAdd_Error_FAILED_TO_RUN_NODE, err)
				break
			}

			if shouldCancel {
				stopNode(anytype)
				return
			}

			name, err := anytype.Textile.Name()
			if err != nil {
				sendAccountAddEvent(index, nil, pb.AccountAdd_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
				stopNode(anytype)
				return
			}

			newAcc := &pb.Account{Id: account.Address(), Name: name}
			instance.localAccounts = append(instance.localAccounts, newAcc)

			if shouldCancel {
				stopNode(anytype)
				return
			}

			newAcc.Avatar, err = anytype.Textile.Avatar()
			if err != nil {
				sendAccountAddEvent(index, nil, pb.AccountAdd_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
				stopNode(anytype)
				return
			}

			if shouldCancel {
				stopNode(anytype)
				return
			}

			stopNode(anytype)

			sendAccountAddEvent(index, newAcc, pb.AccountAdd_Error_NULL, nil)

			if shouldCancel {
				return
			}
			index++
		}
	}()

	return response(pb.WalletRecoverResponse_Error_NULL, nil)
}
