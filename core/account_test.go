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
	resp := mw.WalletCreate(&pb.WalletCreateRequest{RootPath: rootPath})
	require.Equal(t, pb.WalletCreateResponse_Error_NULL, resp.Error.Code, resp.Error.Code, "WalletCreate error should be 0")
	require.Equal(t, 12, len(strings.Split(resp.Mnemonic, " ")), "WalletCreate should return 12 words")
	return mw
}

func recoverWallet(t *testing.T, mnemonic string) *Middleware {
	mw := &Middleware{}
	rootPath := os.TempDir()
	resp := mw.WalletRecover(&pb.WalletRecoverRequest{RootPath: rootPath, Mnemonic: mnemonic})
	require.Equal(t, pb.WalletRecoverResponse_Error_NULL, resp.Error.Code, "WalletRecover error should be 0")
	return mw
}

func Test_AccountCreate(t *testing.T) {
	mw := createWallet(t)

	accountCreateResp := mw.AccountCreate(&pb.AccountCreateRequest{Name: "name_test", Avatar: &pb.AccountCreateRequest_AvatarLocalPath{"testdata/pic1.jpg"}})
	require.Equal(t, "name_test", accountCreateResp.Account.Name, "AccountCreateResponse has account with wrong name '%s'", accountCreateResp.Account.Name)

	imageGetBlobResp := mw.ImageGetBlob(&pb.ImageGetBlobRequest{Id: accountCreateResp.Account.Avatar.GetImage().Id, Size_: pb.ImageSize_SMALL})
	require.Equal(t, pb.ImageGetBlobResponse_Error_NULL, imageGetBlobResp.Error.Code, "ImageGetBlobResponse contains error: %+v", imageGetBlobResp.Error)
	require.True(t, len(imageGetBlobResp.Blob) > 0, "ava size should be greater than 0")

	err := mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_AccountRecover_LocalWithoutRestart(t *testing.T) {
	mw := createWallet(t)

	accountCreateResp := mw.AccountCreate(&pb.AccountCreateRequest{Name: "name_to_test_recover", Avatar: &pb.AccountCreateRequest_AvatarLocalPath{"testdata/pic1.jpg"}})
	require.Equal(t, "name_to_test_recover", accountCreateResp.Account.Name, "AccountCreateResponse has account with wrong name '%s'", accountCreateResp.Account.Name)

	err := mw.Stop()
	require.NoError(t, err, "failed to stop node")

	var accountCh = make(chan *pb.Account, 1)
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_AccountShow); ok {
			if aa.AccountShow.Index != 0 {
				return
			}

			accountCh <- aa.AccountShow.Account
		}
	}

	walletRecoverResp := mw.WalletRecover(&pb.WalletRecoverRequest{RootPath: mw.rootPath, Mnemonic: mw.mnemonic})
	require.Equal(t, pb.WalletRecoverResponse_Error_NULL, walletRecoverResp.Error.Code, "WalletRecoverResponse contains error: %+v", walletRecoverResp.Error)

	accountRecoverResp := mw.AccountRecover(&pb.AccountRecoverRequest{})
	require.Equal(t, pb.AccountRecoverResponse_Error_NULL, accountRecoverResp.Error.Code, "AccountRecoverResponse contains error: %+v", accountRecoverResp.Error)

	var account *pb.Account
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

	accountCreateResp := mw.AccountCreate(&pb.AccountCreateRequest{Name: "name_to_test_recover", Avatar: &pb.AccountCreateRequest_AvatarLocalPath{"testdata/pic1.jpg"}})
	require.Equal(t, "name_to_test_recover", accountCreateResp.Account.Name, "AccountCreateResponse has account with wrong name '%s'", accountCreateResp.Account.Name)

	err := mw.Stop()
	require.NoError(t, err, "failed to stop node")
	rootPath := mw.rootPath
	mnemonic := mw.mnemonic

	// reset singleton to emulate restart
	mw = &Middleware{}

	var accountCh = make(chan *pb.Account, 1)
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_AccountShow); ok {
			if aa.AccountShow.Index != 0 {
				return
			}

			accountCh <- aa.AccountShow.Account
		}
	}

	walletRecoverResp := mw.WalletRecover(&pb.WalletRecoverRequest{RootPath: rootPath, Mnemonic: mnemonic})
	require.Equal(t, pb.WalletRecoverResponse_Error_NULL, walletRecoverResp.Error.Code, "WalletRecoverResponse contains error: %+v", walletRecoverResp.Error)

	accountRecoverResp := mw.AccountRecover(&pb.AccountRecoverRequest{})
	require.Equal(t, pb.AccountRecoverResponse_Error_NULL, accountRecoverResp.Error.Code, "AccountRecoverResponse contains error: %+v", accountRecoverResp.Error)

	var account *pb.Account
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

	var account *pb.Account
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_AccountShow); ok {
			account = aa.AccountShow.Account
		}
	}

	accountRecoverResp := mw.AccountRecover(&pb.AccountRecoverRequest{})
	require.Equal(t, pb.AccountRecoverResponse_Error_NO_ACCOUNTS_FOUND, accountRecoverResp.Error.Code, "AccountRecoverResponse contains error: %+v", accountRecoverResp.Error)

	require.Nil(t, account, "account shouldn't be recovered")

	err := mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_RecoverRemoteExisting(t *testing.T) {
	mw := recoverWallet(t, "input blame switch simple fatigue fragile grab goose unusual identify abuse use")
	require.Equal(t, len(mw.localAccounts), 0, "localAccounts should be empty, instead got length = %d", len(mw.localAccounts))

	var accountCh = make(chan *pb.Account, 1)
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_AccountShow); ok {
			if aa.AccountShow.Index != 0 {
				return
			}

			accountCh <- aa.AccountShow.Account
		}
	}

	accountRecoverResp := mw.AccountRecover(&pb.AccountRecoverRequest{})
	require.Equal(t, pb.AccountRecoverResponse_Error_NULL, accountRecoverResp.Error.Code, "AccountRecoverResponse contains error: %+v", accountRecoverResp.Error)

	var account *pb.Account
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
