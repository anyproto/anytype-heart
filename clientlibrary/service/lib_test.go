package service

import (
	"fmt"
	"testing"

	"github.com/gogo/protobuf/proto"

	"github.com/anyproto/anytype-heart/pb"
)

func TestUnpack(t *testing.T) {
	b, _ := proto.Marshal(&pb.RpcWalletRecoverResponse{})

	var msg pb.RpcWalletRecoverResponse
	err := proto.Unmarshal(b, &msg)
	if err != nil {
		fmt.Println(err.Error())
	}
}
