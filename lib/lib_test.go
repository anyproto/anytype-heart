package main

import (
	"fmt"
	"os"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
)

func SignUp(t *testing.T) {
	rootPath := os.TempDir()
	msg1, err := proto.Marshal(&pb.WalletCreateRequest{RootPath: rootPath})
	fmt.Printf("rootPath: %s\n", rootPath)
	require.NoError(t, err, "failed to marshal WalletCreateRequest")

	resp1 := WalletCreate(msg1)

	var msg2 pb.WalletCreateResponse
	err = proto.Unmarshal(resp1, &msg2)
	require.NoError(t, err, "failed to unmarshal WalletCreateResponse")

	msg3, err := proto.Marshal(&pb.AccountCreateRequest{Username: "testname", AvatarLocalPath: "testdata/pic1.jpg"})
	require.NoError(t, err, "failed to marshal AccountCreateRequest")

	resp2 := AccountCreate(msg3)
	var msg4 pb.AccountCreateResponse
	err = proto.Unmarshal(resp2, &msg4)
	require.NoError(t, err, "failed to unmarshal AccountCreateResponse")

	msg5, err := proto.Marshal(&pb.ImageGetBlobRequest{Id: msg4.Account.Avatar.Id, Size: pb.ImageSize_SMALL})
	if err != nil {
		require.NoError(t, err, "failed to marshal AccountCreateRequest")
	}
	resp3 := ImageGetBlob(msg5)
	var msg6 pb.ImageGetBlobResponse
	err = proto.Unmarshal(resp3, &msg6)
	require.NoError(t, err, "failed to unmarshal ImageGetBlobResponse")
	require.Equal(t, msg6.Error.Code, pb.ImageGetBlobResponse_Error_NULL, "ImageGetBlobResponse contains error: %s", msg6.Error.Code.String())
	require.True(t, len(msg6.Blob) > 0, "ava size should be greater than 0")

}
