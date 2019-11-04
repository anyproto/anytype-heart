package core

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/require"
)

func createWallet(t *testing.T) *Middleware {
	mw := &Middleware{}
	rootPath := os.TempDir()
	resp := mw.WalletCreate(&pb.Rpc_Wallet_Create_Request{RootPath: rootPath})
	require.Equal(t, pb.Rpc_Wallet_Create_Response_Error_NULL, resp.Error.Code, resp.Error.Code, "WalletCreate error should be 0")
	require.Equal(t, 12, len(strings.Split(resp.Mnemonic, " ")), "WalletCreate should return 12 words")
	return mw
}

func recoverWallet(t *testing.T, mnemonic string) *Middleware {
	mw := &Middleware{}
	rootPath := os.TempDir()
	resp := mw.WalletRecover(&pb.Rpc_Wallet_Recover_Request{RootPath: rootPath, Mnemonic: mnemonic})
	require.Equal(t, pb.Rpc_Wallet_Recover_Response_Error_NULL, resp.Error.Code, "WalletRecover error should be 0")
	return mw
}

func Test_AccountCreate(t *testing.T) {
	mw := createWallet(t)

	accountCreateResp := mw.AccountCreate(&pb.Rpc_Account_Create_Request{Name: "name_test", Avatar: &pb.Rpc_Account_Create_Request_AvatarLocalPath{"testdata/pic1.jpg"}})
	require.Equal(t, "name_test", accountCreateResp.Account.Name, "AccountCreate_Response has account with wrong name '%s'", accountCreateResp.Account.Name)

	imageGetBlobResp := mw.ImageGetBlob(&pb.Rpc_Image_Get_Blob_Request{Id: accountCreateResp.Account.Avatar.GetImage().Id, Size_: pb.ImageSize_SMALL})
	require.Equal(t, pb.Rpc_Image_Get_Blob_Response_Error_NULL, imageGetBlobResp.Error.Code, "ImageGetBlob_Response contains error: %+v", imageGetBlobResp.Error)
	require.True(t, len(imageGetBlobResp.Blob) > 0, "ava size should be greater than 0")

	err := mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_AccountRecover_LocalWithoutRestart(t *testing.T) {
	mw := createWallet(t)

	accountCreateResp := mw.AccountCreate(&pb.Rpc_Account_Create_Request{Name: "name_to_test_recover", Avatar: &pb.Rpc_Account_Create_Request_AvatarLocalPath{"testdata/pic1.jpg"}})
	require.Equal(t, "name_to_test_recover", accountCreateResp.Account.Name, "AccountCreate_Response has account with wrong name '%s'", accountCreateResp.Account.Name)

	err := mw.Stop()
	require.NoError(t, err, "failed to stop node")

	var accountCh = make(chan *pb.Model_Account, 10)
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_Account_Show); ok {
			if aa.AccountShow.Index != 0 {
				return
			}

			accountCh <- aa.AccountShow.Account
		}
	}

	walletRecoverResp := mw.WalletRecover(&pb.Rpc_Wallet_Recover_Request{RootPath: mw.rootPath, Mnemonic: mw.mnemonic})
	require.Equal(t, pb.Rpc_Wallet_Recover_Response_Error_NULL, walletRecoverResp.Error.Code, "WalletRecover_Response contains error: %+v", walletRecoverResp.Error)

	accountRecoverResp := mw.AccountRecover(&pb.Rpc_Account_Recover_Request{})
	require.Equal(t, pb.Rpc_Account_Recover_Response_Error_NULL, accountRecoverResp.Error.Code, "AccountRecover_Response contains error: %+v", accountRecoverResp.Error)

	var account *pb.Rpc_Account_
	select {
	case account = <-accountCh:
		break
	case <-time.After(time.Minute):
		break
	}
	require.NotNil(t, account, "didn't receive event with 0 account")

	err = mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_AccountRecover_LocalAfterRestart(t *testing.T) {
	mw := createWallet(t)

	accountCreateResp := mw.AccountCreate(&pb.Rpc_Account_Create_Request{Name: "name_to_test_recover", Avatar: &pb.Rpc_Account_Create_Request_AvatarLocalPath{"testdata/pic1.jpg"}})
	require.Equal(t, "name_to_test_recover", accountCreateResp.Account.Name, "AccountCreate_Response has account with wrong name '%s'", accountCreateResp.Account.Name)

	err := mw.Stop()
	require.NoError(t, err, "failed to stop node")
	rootPath := mw.rootPath
	mnemonic := mw.mnemonic

	// reset singleton to emulate restart
	mw = &Middleware{}

	var accountCh = make(chan *pb.Model_Account, 10)
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_Account_Show); ok {
			if aa.AccountShow.Index != 0 {
				return
			}

			accountCh <- aa.AccountShow.Account
		}
	}

	walletRecoverResp := mw.WalletRecover(&pb.Rpc_Wallet_Recover_Request{RootPath: rootPath, Mnemonic: mnemonic})
	require.Equal(t, pb.Rpc_Wallet_Recover_Response_Error_NULL, walletRecoverResp.Error.Code, "WalletRecover_Response contains error: %+v", walletRecoverResp.Error)

	accountRecoverResp := mw.AccountRecover(&pb.Rpc_Account_Recover_Request{})
	require.Equal(t, pb.Rpc_Account_Recover_Response_Error_NULL, accountRecoverResp.Error.Code, "AccountRecover_Response contains error: %+v", accountRecoverResp.Error)

	var account *pb.Rpc_Account_
	select {
	case account = <-accountCh:
		break
	case <-time.After(time.Minute):
		break
	}

	require.NotNil(t, account, "didn't receive event with 0 account")

	err = mw.Stop()
}

func Test_AccountRecover_RemoteNotExisting(t *testing.T) {
	mw := recoverWallet(t, "limit oxygen february destroy subway toddler umbrella nose praise shield afford eager")
	require.Equal(t, len(mw.localAccounts), 0, "localAccounts should be empty, instead got length = %d", len(mw.localAccounts))

	var account *pb.Rpc_Account_
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_Account_Show); ok {
			account = aa.AccountShow.Account
		}
	}

	accountRecoverResp := mw.AccountRecover(&pb.Rpc_Account_Recover_Request{})
	require.Equal(t, pb.Rpc_Account_Recover_Response_Error_NO_ACCOUNTS_FOUND, accountRecoverResp.Error.Code, "AccountRecover_Response contains error: %+v", accountRecoverResp.Error)

	require.Nil(t, account, "account shouldn't be recovered")

	err := mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_RecoverRemoteExisting(t *testing.T) {
	mw := recoverWallet(t, "input blame switch simple fatigue fragile grab goose unusual identify abuse use")
	require.Equal(t, len(mw.localAccounts), 0, "localAccounts should be empty, instead got length = %d", len(mw.localAccounts))

	var accountCh = make(chan *pb.Model_Account, 10)
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_Account_Show); ok {
			if aa.AccountShow.Index != 0 {
				return
			}

			accountCh <- aa.AccountShow.Account
		}
	}

	accountRecoverResp := mw.AccountRecover(&pb.Rpc_Account_Recover_Request{})
	require.Equal(t, pb.Rpc_Account_Recover_Response_Error_NULL, accountRecoverResp.Error.Code, "AccountRecover_Response contains error: %+v", accountRecoverResp.Error)

	var account *pb.Rpc_Account_
	select {
	case account = <-accountCh:
		break
	case <-time.After(time.Minute):
		break
	}

	require.NotNil(t, account, "account should be found")
	require.Equal(t, "name_to_test_recover", account.Name)
	err := mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}
