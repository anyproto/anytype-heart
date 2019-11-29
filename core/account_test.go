package core

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/stretchr/testify/require"
)

func createWallet(t *testing.T) *Middleware {
	mw := &Middleware{}
	rootPath := os.TempDir()
	resp := mw.WalletCreate(&pb.RpcWalletCreateRequest{RootPath: rootPath})
	require.Equal(t, pb.RpcWalletCreateResponseError_NULL, resp.Error.Code, resp.Error.Code, "WalletCreate error should be 0")
	require.Equal(t, 12, len(strings.Split(resp.Mnemonic, " ")), "WalletCreate should return 12 words")
	return mw
}

func recoverWallet(t *testing.T, mnemonic string) *Middleware {
	mw := &Middleware{}
	rootPath := os.TempDir()
	resp := mw.WalletRecover(&pb.RpcWalletRecoverRequest{RootPath: rootPath, Mnemonic: mnemonic})
	require.Equal(t, pb.RpcWalletRecoverResponseError_NULL, resp.Error.Code, "WalletRecover error should be 0")
	return mw
}

func TestAccountCreate(t *testing.T) {
	mw := createWallet(t)

	accountCreateResp := mw.AccountCreate(&pb.RpcAccountCreateRequest{Name: "name_test", Avatar: &pb.RpcAccountCreateRequestAvatarOfAvatarLocalPath{"testdata/pic1.jpg"}})
	require.Equal(t, "name_test", accountCreateResp.Account.Name, "AccountCreateResponse has account with wrong name '%s'", accountCreateResp.Account.Name)

	imageGetBlobResp := mw.ImageGetBlob(&pb.RpcIpfsImageGetBlobRequest{Id: accountCreateResp.Account.Avatar.GetImage().Id, Size_: model.Image_Small})
	require.Equal(t, pb.RpcIpfsImageGetBlobResponseError_NULL, imageGetBlobResp.Error.Code, "ImageGetBlobResponse contains error: %+v", imageGetBlobResp.Error)
	require.True(t, len(imageGetBlobResp.Blob) > 0, "ava size should be greater than 0")

	err := mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func TestAccountRecoverLocalWithoutRestart(t *testing.T) {
	mw := createWallet(t)

	accountCreateResp := mw.AccountCreate(&pb.RpcAccountCreateRequest{Name: "name_to_test_recover", Avatar: &pb.RpcAccountCreateRequestAvatarOfAvatarLocalPath{"testdata/pic1.jpg"}})
	require.Equal(t, "name_to_test_recover", accountCreateResp.Account.Name, "AccountCreateResponse has account with wrong name '%s'", accountCreateResp.Account.Name)

	err := mw.Stop()
	require.NoError(t, err, "failed to stop node")

	var accountCh = make(chan *model.Account, 10)
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Messages[0].Value.(*pb.EventMessageValueOfAccountShow); ok {
			if aa.AccountShow.Index != 0 {
				return
			}

			accountCh <- aa.AccountShow.Account
		}
	}

	walletRecoverResp := mw.WalletRecover(&pb.RpcWalletRecoverRequest{RootPath: mw.rootPath, Mnemonic: mw.mnemonic})
	require.Equal(t, pb.RpcWalletRecoverResponseError_NULL, walletRecoverResp.Error.Code, "WalletRecoverResponse contains error: %+v", walletRecoverResp.Error)

	accountRecoverResp := mw.AccountRecover(&pb.RpcAccountRecoverRequest{})
	require.Equal(t, pb.RpcAccountRecoverResponseError_NULL, accountRecoverResp.Error.Code, "AccountRecoverResponse contains error: %+v", accountRecoverResp.Error)

	var account *model.Account
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

func TestAccountRecoverLocalAfterRestart(t *testing.T) {
	mw := createWallet(t)

	accountCreateResp := mw.AccountCreate(&pb.RpcAccountCreateRequest{Name: "name_to_test_recover", Avatar: &pb.RpcAccountCreateRequestAvatarOfAvatarLocalPath{"testdata/pic1.jpg"}})
	require.Equal(t, "name_to_test_recover", accountCreateResp.Account.Name, "AccountCreateResponse has account with wrong name '%s'", accountCreateResp.Account.Name)

	err := mw.Stop()
	require.NoError(t, err, "failed to stop node")
	rootPath := mw.rootPath
	mnemonic := mw.mnemonic

	// reset singleton to emulate restart
	mw = &Middleware{}

	var accountCh = make(chan *model.Account, 10)
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Messages[0].Value.(*pb.EventMessageValueOfAccountShow); ok {
			if aa.AccountShow.Index != 0 {
				return
			}

			accountCh <- aa.AccountShow.Account
		}
	}

	walletRecoverResp := mw.WalletRecover(&pb.RpcWalletRecoverRequest{RootPath: rootPath, Mnemonic: mnemonic})
	require.Equal(t, pb.RpcWalletRecoverResponseError_NULL, walletRecoverResp.Error.Code, "WalletRecoverResponse contains error: %+v", walletRecoverResp.Error)

	accountRecoverResp := mw.AccountRecover(&pb.RpcAccountRecoverRequest{})
	require.Equal(t, pb.RpcAccountRecoverResponseError_NULL, accountRecoverResp.Error.Code, "AccountRecoverResponse contains error: %+v", accountRecoverResp.Error)

	var account *model.Account
	select {
	case account = <-accountCh:
		break
	case <-time.After(time.Minute):
		break
	}

	require.NotNil(t, account, "didn't receive event with 0 account")

	err = mw.Stop()
}

func TestAccountRecoverRemoteNotExisting(t *testing.T) {
	mw := recoverWallet(t, "limit oxygen february destroy subway toddler umbrella nose praise shield afford eager")
	require.Equal(t, len(mw.localAccounts), 0, "localAccounts should be empty, instead got length = %d", len(mw.localAccounts))

	var account *model.Account
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Messages[0].Value.(*pb.EventMessageValueOfAccountShow); ok {
			account = aa.AccountShow.Account
		}
	}

	accountRecoverResp := mw.AccountRecover(&pb.RpcAccountRecoverRequest{})
	require.Equal(t, pb.RpcAccountRecoverResponseError_NO_ACCOUNTS_FOUND, accountRecoverResp.Error.Code, "AccountRecoverResponse contains error: %+v", accountRecoverResp.Error)

	require.Nil(t, account, "account shouldn't be recovered")

	err := mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func TestRecoverRemoteExisting(t *testing.T) {
	mw := recoverWallet(t, "salt rug lawsuit three agent battle impact human monster mechanic width shoulder")
	require.Equal(t, len(mw.localAccounts), 0, "localAccounts should be empty, instead got length = %d", len(mw.localAccounts))

	var accountCh = make(chan *model.Account, 10)
	mw.SendEvent = func(event *pb.Event) {
		if aa, ok := event.Messages[0].Value.(*pb.EventMessageValueOfAccountShow); ok {
			if aa.AccountShow.Index != 0 {
				return
			}

			accountCh <- aa.AccountShow.Account
		}
	}

	accountRecoverResp := mw.AccountRecover(&pb.RpcAccountRecoverRequest{})
	require.Equal(t, pb.RpcAccountRecoverResponseError_NULL, accountRecoverResp.Error.Code, "AccountRecoverResponse contains error: %+v", accountRecoverResp.Error)

	var account *model.Account
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
