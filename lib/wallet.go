package main

import (
	"github.com/anytypeio/go-anytype-library/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
)

const wordCount int = 12

//todo: move some logic to the backend lib?
//exportMobile WalletCreate
func WalletCreate(b []byte) []byte {
	response := func(mnemonic string, code pb.WalletCreateR_Error_Code, err error) []byte {
		m := &pb.WalletCreateR{Mnemonic: mnemonic, Error: &pb.WalletCreateR_Error{Code: code}}
		if err != nil {
			m.Error.Desc = err.Error()
		}

		return Marshal(m)
	}

	var q pb.WalletCreateQ
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response("", pb.WalletCreateR_Error_BAD_INPUT, err)
	}

	mnemonic, err := core.WalletGenerateMnemonic(wordCount)
	if err != nil {
		return response("", pb.WalletCreateR_Error_UNKNOWN_ERROR, err)
	}

	instance.mnemonic = mnemonic
	instance.rootPath = q.RootPath

	return response(mnemonic, pb.WalletCreateR_Error_NULL, nil)
}

//exportMobile WalletRecover
func WalletRecover(b []byte) []byte {
	response := func(code pb.WalletRecoverR_Error_Code, err error) []byte {
		m := &pb.WalletRecoverR{Error: &pb.WalletRecoverR_Error{Code: code}}
		if err != nil {
			m.Error.Desc = err.Error()
		}

		return Marshal(m)
	}

	sendAccountAddEvent := func(index int, account *pb.Account, code pb.AccountAdd_Error_Code, err error) {
		pbErr := &pb.AccountAdd_Error{Code: code}
		m := &pb.Event{Message: &pb.Event_AccountAdd{AccountAdd: &pb.AccountAdd{Index: int64(index), Account: account, Error: pbErr}}}
		if err != nil {
			pbErr.Desc = err.Error()
		}

		SendEvent(m)
	}

	var q pb.WalletRecoverQ
	err := proto.Unmarshal(b, &q)
	if err != nil {
		return response(pb.WalletRecoverR_Error_BAD_INPUT, err)
	}

	// test if mnemonic is correct
	_, err = core.WalletAccountAt(q.Mnemonic, 0, "")
	if err != nil {
		return response(pb.WalletRecoverR_Error_BAD_INPUT, err)
	}

	go func() {
		index :=0
		for {
			account, err := core.WalletAccountAt(q.Mnemonic, index, "")
			if err != nil {
				sendAccountAddEvent(index, nil, pb.AccountAdd_Error_BAD_INPUT, err)
				break
			}

			err = core.WalletInitRepo(instance.rootPath, account.Seed())
			if err != nil && err != core.ErrRepoExists {
				sendAccountAddEvent(index,nil, pb.AccountAdd_Error_FAILED_TO_CREATE_LOCAL_REPO, err)
				break
			}

			anytype, err := core.New(instance.rootPath, account.Address())
			if err != nil {
				sendAccountAddEvent(index,nil, pb.AccountAdd_Error_UNKNOWN_ERROR, err)
				break
			}

			err = instance.Run()
			if err != nil {
				if index != 0 {
					return
				}

				if err == core.ErrRepoCorrupted {
					sendAccountAddEvent(index,nil, pb.AccountAdd_Error_LOCAL_REPO_EXISTS_BUT_CORRUPTED, err)
				}

				sendAccountAddEvent(index,nil, pb.AccountAdd_Error_FAILED_TO_RUN_NODE, err)
			}

			name, err := anytype.Textile.Name()
			if err != nil {
				sendAccountAddEvent(index,nil, pb.AccountAdd_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
				break
			}

			avatar, err := anytype.Textile.Avatar()
			if err != nil {
				sendAccountAddEvent(index,nil, pb.AccountAdd_Error_FAILED_TO_FIND_ACCOUNT_INFO, err)
				break
			}

			err = anytype.Textile.Node().Stop()
			if err != nil {
				sendAccountAddEvent(index,nil, pb.AccountAdd_Error_UNKNOWN_ERROR, err)
				break
			}

			sendAccountAddEvent(index, &pb.Account{Id: account.Address(), Name: name, Avatar: avatar}, pb.AccountAdd_Error_NULL, nil)
			index ++
		}
	}()

	return response(pb.WalletRecoverR_Error_NULL, nil)
}
