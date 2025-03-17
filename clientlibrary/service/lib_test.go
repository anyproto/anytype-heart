package service

import (
	"fmt"
	"testing"

	"github.com/anyproto/anytype-heart/pb"
)

func TestUnpack(t *testing.T) {
	response := pb.RpcWalletRecoverResponse{}
	b, _ := response.MarshalVT()

	var msg pb.RpcWalletRecoverResponse
	err := msg.UnmarshalVT(b)
	if err != nil {
		fmt.Println(err.Error())
	}
}
