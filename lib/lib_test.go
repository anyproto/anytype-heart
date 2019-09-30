package main

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
)

func Test_SignUp(t *testing.T) {
	rootPath := os.TempDir()
	walletCreateReq, err := proto.Marshal(&pb.WalletCreateRequest{RootPath: rootPath})
	require.NoError(t, err, "failed to marshal WalletCreateRequest")

	walletCreateResp := WalletCreate(walletCreateReq)
	var walletCreateRespMsg pb.WalletCreateResponse
	err = proto.Unmarshal(walletCreateResp, &walletCreateRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletCreateResponse")
	fmt.Printf("rootPath: %s\nmnemonic:%s\n", rootPath, walletCreateRespMsg.Mnemonic)

	accountCreateReq, err := proto.Marshal(&pb.AccountCreateRequest{Username: "name_to_test_recover", AvatarLocalPath: "testdata/pic1.jpg"})
	require.NoError(t, err, "failed to marshal AccountCreateRequest")

	accountCreateResp := AccountCreate(accountCreateReq)
	var accountCreateRespMsg pb.AccountCreateResponse
	err = proto.Unmarshal(accountCreateResp, &accountCreateRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountCreateResponse")
	require.Equal(t, pb.AccountCreateResponse_Error_NULL, accountCreateRespMsg.Error.Code, "AccountCreateResponse contains error: %s %s", accountCreateRespMsg.Error.Code.String(), accountCreateRespMsg.Error.Description)
	require.Equal(t, "name_to_test_recover", accountCreateRespMsg.Account.Name, "ImageGetBlobResponse got account with name '%s'", accountCreateRespMsg.Account.Name)

	imageGetBlobReq, err := proto.Marshal(&pb.ImageGetBlobRequest{Id: accountCreateRespMsg.Account.Avatar.Id, Size: pb.ImageSize_SMALL})
	if err != nil {
		require.NoError(t, err, "failed to marshal AccountCreateRequest")
	}
	imageGetBlobResp := ImageGetBlob(imageGetBlobReq)
	var imageGetBlobRespMsg pb.ImageGetBlobResponse
	err = proto.Unmarshal(imageGetBlobResp, &imageGetBlobRespMsg)
	require.NoError(t, err, "failed to unmarshal ImageGetBlobResponse")
	require.Equal(t, pb.ImageGetBlobResponse_Error_NULL, imageGetBlobRespMsg.Error.Code, "ImageGetBlobResponse contains error: %s", imageGetBlobRespMsg.Error.Code.String())
	require.True(t, len(imageGetBlobRespMsg.Blob) > 0, "ava size should be greater than 0")

	time.Sleep(time.Minute*3)
	err = instance.Stop()
	require.NoError(t, err, "failed to stop instance")
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

	accountCreateReq, err := proto.Marshal(&pb.AccountCreateRequest{Username: "testname", AvatarLocalPath: "testdata/pic1.jpg"})
	require.NoError(t, err, "failed to marshal AccountCreateRequest")

	accountCreateResp := AccountCreate(accountCreateReq)
	var accountCreateRespMsg pb.AccountCreateResponse
	err = proto.Unmarshal(accountCreateResp, &accountCreateRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountCreateResponse")

	err = instance.Stop()
	require.NoError(t, err, "failed to stop node")

	walletRecoverReq, err := proto.Marshal(&pb.WalletRecoverRequest{RootPath: rootPath, Mnemonic: walletCreateRespMsg.Mnemonic})
	fmt.Printf("rootPath: %s\n", rootPath)
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	walletRecoverResp := WalletRecover(walletRecoverReq)
	var walletRecoverRespMsg pb.WalletRecoverResponse
	err = proto.Unmarshal(walletRecoverResp, &walletRecoverRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletRecoverResponse")
	require.Equal(t, pb.WalletRecoverResponse_Error_NULL, walletRecoverRespMsg.Error.Code, "WalletRecoverResponse contains error: %s %s", walletRecoverRespMsg.Error.Code, walletRecoverRespMsg.Error.Description)

	time.Sleep(time.Second * 10)

	accountSelectReq, err := proto.Marshal(&pb.AccountSelectRequest{Index: 0})
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	accountSelectResp := AccountSelect(accountSelectReq)
	var accountSelectRespMsg pb.AccountSelectResponse
	err = proto.Unmarshal(accountSelectResp, &accountSelectRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountSelectResponse")
	require.Equal(t, pb.AccountSelectResponse_Error_NULL, accountSelectRespMsg.Error.Code, "AccountSelectResponse contains error: %s %s", accountSelectRespMsg.Error.Code, accountSelectRespMsg.Error.Description)

	err = instance.Stop()
	require.NoError(t, err, "failed to stop instance")
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

	accountCreateReq, err := proto.Marshal(&pb.AccountCreateRequest{Username: "testname", AvatarLocalPath: "testdata/pic1.jpg"})
	require.NoError(t, err, "failed to marshal AccountCreateRequest")

	accountCreateResp := AccountCreate(accountCreateReq)
	var accountCreateRespMsg pb.AccountCreateResponse
	err = proto.Unmarshal(accountCreateResp, &accountCreateRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountCreateResponse")

	err = instance.Stop()
	require.NoError(t, err, "failed to stop node")

	instance = &Instance{}

	walletRecoverReq, err := proto.Marshal(&pb.WalletRecoverRequest{RootPath: rootPath, Mnemonic: walletCreateRespMsg.Mnemonic})
	fmt.Printf("rootPath: %s\n", rootPath)
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	walletRecoverResp := WalletRecover(walletRecoverReq)
	var walletRecoverRespMsg pb.WalletRecoverResponse
	err = proto.Unmarshal(walletRecoverResp, &walletRecoverRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletRecoverResponse")
	require.Equal(t, pb.WalletRecoverResponse_Error_NULL, walletRecoverRespMsg.Error.Code, "WalletRecoverResponse contains error: %s %s", walletRecoverRespMsg.Error.Code, walletRecoverRespMsg.Error.Description)

	time.Sleep(time.Second * 10)

	accountSelectReq, err := proto.Marshal(&pb.AccountSelectRequest{Index: 0})
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	accountSelectResp := AccountSelect(accountSelectReq)
	var accountSelectRespMsg pb.AccountSelectResponse
	err = proto.Unmarshal(accountSelectResp, &accountSelectRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountSelectResponse")
	require.Equal(t, pb.AccountSelectResponse_Error_NULL, accountSelectRespMsg.Error.Code, "AccountSelectResponse contains error: %s %s", accountSelectRespMsg.Error.Code, accountSelectRespMsg.Error.Description)

	err = instance.Stop()
	require.NoError(t, err, "failed to stop instance")
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
	require.Equal(t, pb.WalletRecoverResponse_Error_NULL, walletRecoverRespMsg.Error.Code, "WalletRecoverResponse contains error: %s %s", walletRecoverRespMsg.Error.Code, walletRecoverRespMsg.Error.Description)

	time.Sleep(time.Second * 30)

	require.Equal(t, len(instance.localAccounts), 0, "localAccounts should be empty, instead got length = %d", len(instance.localAccounts))

	accountSelectReq, err := proto.Marshal(&pb.AccountSelectRequest{Index: 0})
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	accountSelectResp := AccountSelect(accountSelectReq)
	var accountSelectRespMsg pb.AccountSelectResponse
	err = proto.Unmarshal(accountSelectResp, &accountSelectRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountSelectResponse")
	require.Equal(t, pb.AccountSelectResponse_Error_NULL, accountSelectRespMsg.Error.Code, "AccountSelectResponse contains error: %s %s", accountSelectRespMsg.Error.Code, accountSelectRespMsg.Error.Description)

	err = instance.Stop()
	require.NoError(t, err, "failed to stop instance")
}

func Test_RecoverRemoteExisting(t *testing.T) {
	rootPath := os.TempDir()

	walletRecoverReq, err := proto.Marshal(&pb.WalletRecoverRequest{RootPath: rootPath, Mnemonic: "input blame switch simple fatigue fragile grab goose unusual identify abuse use"})
	fmt.Printf("rootPath: %s\n", rootPath)
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	walletRecoverResp := WalletRecover(walletRecoverReq)
	var walletRecoverRespMsg pb.WalletRecoverResponse
	err = proto.Unmarshal(walletRecoverResp, &walletRecoverRespMsg)
	require.NoError(t, err, "failed to unmarshal WalletRecoverResponse")
	require.Equal(t, pb.WalletRecoverResponse_Error_NULL, walletRecoverRespMsg.Error.Code, "WalletRecoverResponse contains error: %s %s", walletRecoverRespMsg.Error.Code)
	start := time.Now()
	for {
		if time.Since(start).Seconds()>300{
			break
		}

		if len(instance.localAccounts) > 0  {
			fmt.Println("found account!")
			break
		}

		time.Sleep(time.Second)
	}

	require.Equal(t, len(instance.localAccounts), 1, "len(localAccounts) should be 1 , instead got = %d", len(instance.localAccounts))

	accountSelectReq, err := proto.Marshal(&pb.AccountSelectRequest{Index: 0})
	require.NoError(t, err, "failed to marshal WalletRecoverRequest")

	accountSelectResp := AccountSelect(accountSelectReq)
	var accountSelectRespMsg pb.AccountSelectResponse
	err = proto.Unmarshal(accountSelectResp, &accountSelectRespMsg)
	require.NoError(t, err, "failed to unmarshal AccountSelectResponse")
	require.Equal(t, pb.AccountSelectResponse_Error_NULL, accountSelectRespMsg.Error.Code, "AccountSelectResponse contains error: %s %s", accountSelectRespMsg.Error.Code, accountSelectRespMsg.Error.Description)
	require.Equal(t, "name_to_test_recover", accountSelectRespMsg.Account.Name, "AccountSelectResponse should contains account with the name 'name_to_test_recover'")
/*	err = instance.Textile.SnapshotThreads()
	if err != nil {
		fmt.Printf("snaphot failed: %s\n", err.Error())
	}
	time.Sleep(time.Minute)*/
	err = instance.Stop()
	require.NoError(t, err, "failed to stop instance")
}

