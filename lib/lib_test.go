package lib

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
)

func Test_EventHandler(t *testing.T) {
	var eventReceived *pb.Event
	SetEventHandler(func(event *pb.Event) {
		eventReceived = event
	})

	eventSent := &pb.Event{Message: &pb.Event_AccountAdd{AccountAdd: &pb.AccountAdd{Index: 0, Account: &pb.Account{Id: "1", Name: "name"}}}}
	SendEvent(eventSent)

	require.Equal(t, eventSent, eventReceived, "eventReceived not equal to eventSent: %s %s", eventSent, eventReceived)

}

func Test_SignUp(t *testing.T) {
	rootPath := os.TempDir()
	walletCreateReq, err := proto.Marshal(&pb.WalletCreateRequest{RootPath: rootPath})
	require.NoError(t, err, "failed to marshal WalletCreateRequest")

	walletCreateResp := WalletCreate(walletCreateReq)
	var walletCreateRespMsg pb.WalletCreateResponse
	err = proto.Unmarshal(walletCreateResp, &walletCreateRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletCreateResponse")
	fmt.Printf("rootPath: %s\nmnemonic:%s\n", rootPath, walletCreateRespMsg.Mnemonic)

	accountCreateReq, err := proto.Marshal(&pb.AccountCreateRequest{Name: "name_to_test_recover", AvatarLocalPath: "testdata/pic1.jpg"})
	require.NoError(t, err, "failed to marshal AccountCreateRequest")

	accountCreateResp := AccountCreate(accountCreateReq)
	var accountCreateRespMsg pb.AccountCreateResponse
	err = proto.Unmarshal(accountCreateResp, &accountCreateRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountCreateResponse")
	require.Nil(t, accountCreateRespMsg.Error, "AccountCreateResponse contains error: %+v", accountCreateRespMsg.Error)
	require.Equal(t, "name_to_test_recover", accountCreateRespMsg.Account.Name, "ImageGetBlobResponse got account with name '%s'", accountCreateRespMsg.Account.Name)

	imageGetBlobReq, err := proto.Marshal(&pb.ImageGetBlobRequest{Id: accountCreateRespMsg.Account.Avatar.Id, Size: pb.ImageSize_SMALL})
	if err != nil {
		require.NoError(t, err, "failed to marshal AccountCreateRequest")
	}
	imageGetBlobResp := ImageGetBlob(imageGetBlobReq)
	var imageGetBlobRespMsg pb.ImageGetBlobResponse
	err = proto.Unmarshal(imageGetBlobResp, &imageGetBlobRespMsg)
	require.NoError(t, err, "failed to unmarshal ImageGetBlobResponse")
	require.Nil(t, imageGetBlobRespMsg.Error, "ImageGetBlobResponse contains error: %+v", imageGetBlobRespMsg.Error)
	require.True(t, len(imageGetBlobRespMsg.Blob) > 0, "ava size should be greater than 0")

	err = mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_RecoverLocalWithoutRestart(t *testing.T) {
	rootPath := os.TempDir()
	walletCreateReq, err := proto.Marshal(&pb.WalletCreateRequest{RootPath: rootPath})
	fmt.Printf("rootPath: %s\n", rootPath)
	require.NoError(t, err, "failed to marshal WalletCreateRequest")

	walletCreateResp := WalletCreate(walletCreateReq)
	var walletCreateRespMsg pb.WalletCreateResponse
	err = proto.Unmarshal(walletCreateResp, &walletCreateRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletCreateResponse")

	accountCreateReq, err := proto.Marshal(&pb.AccountCreateRequest{Name: "testname", AvatarLocalPath: "testdata/pic1.jpg"})
	require.NoError(t, err, "failed to marshal AccountCreateRequest")

	accountCreateResp := AccountCreate(accountCreateReq)
	var accountCreateRespMsg pb.AccountCreateResponse
	err = proto.Unmarshal(accountCreateResp, &accountCreateRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountCreateResponse")

	err = mw.Stop()
	require.NoError(t, err, "failed to stop node")

	var account *pb.Account
	SetEventHandler(func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_AccountAdd); ok {
			if aa.AccountAdd.Index != 0 {
				return
			}

			account = aa.AccountAdd.Account
		}
	})

	walletRecoverReq, err := proto.Marshal(&pb.WalletRecoverRequest{RootPath: rootPath, Mnemonic: walletCreateRespMsg.Mnemonic})
	fmt.Printf("rootPath: %s\n", rootPath)
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	walletRecoverResp := WalletRecover(walletRecoverReq)
	var walletRecoverRespMsg pb.WalletRecoverResponse
	err = proto.Unmarshal(walletRecoverResp, &walletRecoverRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletRecoverResponse")
	require.Nil(t, walletRecoverRespMsg.Error, "WalletRecoverResponse contains error: %+v", walletRecoverRespMsg.Error)

	accountRecoverReq, err := proto.Marshal(&pb.AccountRecoverRequest{})
	require.NoError(t, err, "failed to marshal AccountRecoverRequest")

	accountRecoverResp := AccountRecover(accountRecoverReq)
	var accountRecoverRespMsg pb.AccountRecoverResponse
	err = proto.Unmarshal(accountRecoverResp, &accountRecoverRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountRecoverResponse")
	require.Nil(t, accountRecoverRespMsg.Error, "AccountRecoverResponse contains error: %+v", accountRecoverRespMsg.Error)

	start := time.Now()
	for {
		if time.Since(start).Seconds() > 100 {
			break
		}

		if account != nil {
			fmt.Println("found account!")
			break
		}

		time.Sleep(time.Second)
	}

	require.NotNil(t, account, "didn't receive event with 0 account")

	accountSelectReq, err := proto.Marshal(&pb.AccountSelectRequest{Id: account.Id})
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	accountSelectResp := AccountSelect(accountSelectReq)
	var accountSelectRespMsg pb.AccountSelectResponse
	err = proto.Unmarshal(accountSelectResp, &accountSelectRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountSelectResponse")
	require.Nil(t, accountSelectRespMsg.Error, "AccountSelectResponse contains error: %+v", accountSelectRespMsg.Error)

	err = mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_RecoverLocalAfterRestart(t *testing.T) {
	rootPath := os.TempDir()
	walletCreateReq, err := proto.Marshal(&pb.WalletCreateRequest{RootPath: rootPath})
	fmt.Printf("rootPath: %s\n", rootPath)
	require.NoError(t, err, "failed to marshal WalletCreateRequest")

	walletCreateResp := WalletCreate(walletCreateReq)
	var walletCreateRespMsg pb.WalletCreateResponse
	err = proto.Unmarshal(walletCreateResp, &walletCreateRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletCreateResponse")

	accountCreateReq, err := proto.Marshal(&pb.AccountCreateRequest{Name: "testname", AvatarLocalPath: "testdata/pic1.jpg"})
	require.NoError(t, err, "failed to marshal AccountCreateRequest")

	accountCreateResp := AccountCreate(accountCreateReq)
	var accountCreateRespMsg pb.AccountCreateResponse
	err = proto.Unmarshal(accountCreateResp, &accountCreateRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountCreateResponse")

	err = mw.Stop()
	require.NoError(t, err, "failed to stop node")

	mw = &middleware{}

	var account *pb.Account
	SetEventHandler(func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_AccountAdd); ok {
			if aa.AccountAdd.Index != 0 {
				return
			}

			account = aa.AccountAdd.Account
		}
	})

	walletRecoverReq, err := proto.Marshal(&pb.WalletRecoverRequest{RootPath: rootPath, Mnemonic: walletCreateRespMsg.Mnemonic})
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	walletRecoverResp := WalletRecover(walletRecoverReq)
	var walletRecoverRespMsg pb.WalletRecoverResponse
	err = proto.Unmarshal(walletRecoverResp, &walletRecoverRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletRecoverResponse")
	require.Nil(t, walletRecoverRespMsg.Error, "WalletRecoverResponse contains error: %+v", walletRecoverRespMsg.Error)

	accountRecoverReq, err := proto.Marshal(&pb.AccountRecoverRequest{})
	require.NoError(t, err, "failed to marshal AccountRecoverRequest")

	accountRecoverResp := AccountRecover(accountRecoverReq)
	var accountRecoverRespMsg pb.AccountRecoverResponse
	err = proto.Unmarshal(accountRecoverResp, &accountRecoverRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountRecoverResponse")
	require.Nil(t, accountRecoverRespMsg.Error, "AccountRecoverResponse contains error: %+v", accountRecoverRespMsg.Error)

	start := time.Now()
	for {
		if time.Since(start).Seconds() > 100 {
			break
		}

		if account != nil {
			fmt.Println("found account!")
			break
		}

		time.Sleep(time.Second)
	}

	require.NotNil(t, account, "didn't receive event with 0 account")

	accountSelectReq, err := proto.Marshal(&pb.AccountSelectRequest{Id: account.Id})
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	accountSelectResp := AccountSelect(accountSelectReq)
	var accountSelectRespMsg pb.AccountSelectResponse
	err = proto.Unmarshal(accountSelectResp, &accountSelectRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountSelectResponse")
	require.Nil(t, accountSelectRespMsg.Error, "AccountSelectResponse contains error: %+v", accountSelectRespMsg.Error)

	err = mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_RecoverRemoteNotExisting(t *testing.T) {
	rootPath := os.TempDir()

	walletRecoverReq, err := proto.Marshal(&pb.WalletRecoverRequest{RootPath: rootPath, Mnemonic: "limit oxygen february destroy subway toddler umbrella nose praise shield afford eager"})
	fmt.Printf("rootPath: %s\n", rootPath)
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	walletRecoverResp := WalletRecover(walletRecoverReq)
	var walletRecoverRespMsg pb.WalletRecoverResponse
	err = proto.Unmarshal(walletRecoverResp, &walletRecoverRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletRecoverResponse")
	require.Nil(t, walletRecoverRespMsg.Error, "WalletRecoverResponse contains error: %+v", walletRecoverRespMsg.Error)

	time.Sleep(time.Second * 10)

	require.Equal(t, len(mw.localAccounts), 0, "localAccounts should be empty, instead got length = %d", len(mw.localAccounts))

	err = mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_RecoverRemoteExisting(t *testing.T) {
	rootPath := os.TempDir()

	var account *pb.Account
	SetEventHandler(func(event *pb.Event) {
		if aa, ok := event.Message.(*pb.Event_AccountAdd); ok {
			if aa.AccountAdd.Index != 0 {
				return
			}

			account = aa.AccountAdd.Account
		}
	})

	walletRecoverReq, err := proto.Marshal(&pb.WalletRecoverRequest{RootPath: rootPath, Mnemonic: "input blame switch simple fatigue fragile grab goose unusual identify abuse use"})
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	walletRecoverResp := WalletRecover(walletRecoverReq)
	var walletRecoverRespMsg pb.WalletRecoverResponse
	err = proto.Unmarshal(walletRecoverResp, &walletRecoverRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletRecoverResponse")
	require.Nil(t, walletRecoverRespMsg.Error, "WalletRecoverResponse contains error: %+v", walletRecoverRespMsg.Error)

	accountRecoverReq, err := proto.Marshal(&pb.AccountRecoverRequest{})
	require.NoError(t, err, "failed to marshal AccountRecoverRequest")

	accountRecoverResp := AccountRecover(accountRecoverReq)
	var accountRecoverRespMsg pb.AccountRecoverResponse
	err = proto.Unmarshal(accountRecoverResp, &accountRecoverRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountRecoverResponse")
	require.Nil(t, accountRecoverRespMsg.Error, "AccountRecoverResponse contains error: %+v", accountRecoverRespMsg.Error)

	start := time.Now()
	for {
		if time.Since(start).Seconds() > 100 {
			break
		}

		if account != nil {
			fmt.Println("found account!")
			break
		}

		time.Sleep(time.Second)
	}

	require.NotNil(t, account, "didn't receive event with first(0-index) account")

	accountSelectReq, err := proto.Marshal(&pb.AccountSelectRequest{Id: account.Id})
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	accountSelectResp := AccountSelect(accountSelectReq)
	var accountSelectRespMsg pb.AccountSelectResponse
	err = proto.Unmarshal(accountSelectResp, &accountSelectRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountSelectResponse")
	require.Nil(t, accountSelectRespMsg.Error, "AccountSelectResponse contains error: %+v", accountSelectRespMsg.Error)
	require.Equal(t, "name_to_test_recover", accountSelectRespMsg.Account.Name, "AccountSelectResponse should contains account with the name 'name_to_test_recover'")
	/*	err = mw.Textile.SnapshotThreads()
		if err != nil {
			fmt.Printf("snaphot failed: %s\n", err.Error())
		}
		time.Sleep(time.Minute)*/
	err = mw.Stop()
	require.NoError(t, err, "failed to stop mw")
}

func Test_GetVersion(t *testing.T) {

	v, err := proto.Marshal(&pb.GetVersionRequest{})
	require.NoError(t, err, "failed to marshal GetVersionRequest")

	getVersion := GetVersion(v)
	var versionRespMsg pb.GetVersionResponse
	err = proto.Unmarshal(getVersion, &versionRespMsg)

	require.NoError(t, err, "failed to unmarshal GetVersionResponse")
	require.Nil(t, versionRespMsg.Error, "GetVersionResponse contains error: %+v", versionRespMsg.Error)
	require.Equal(t, versionRespMsg.Version, Version)

}

func Test_Log(t *testing.T) {
	LogTst(t, pb.LogRequest_DEBUG)
	LogTst(t, pb.LogRequest_ERROR)
	LogTst(t, pb.LogRequest_FATAL)
	LogTst(t, pb.LogRequest_INFO)
	LogTst(t, pb.LogRequest_PANIC)
	LogTst(t, pb.LogRequest_WARNING)
}

func LogTst(t *testing.T, level pb.LogRequest_Level) {
	l, err := proto.Marshal(&pb.LogRequest{Message: "test", Level: level})
	require.NoError(t, err, "failed to marshal LogRequest")
	log := Log(l)
	var logRespMsg pb.LogResponse
	err = proto.Unmarshal(log, &logRespMsg)
	require.NoError(t, err, "failed to unmarshal LogResponse")
	require.Nil(t, logRespMsg.Error, "GetVersionResponse contains error: %+v", logRespMsg.Error)
}
