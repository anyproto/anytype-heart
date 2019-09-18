package main

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/requilence/go-anytype/mobile"
	"github.com/requilence/go-anytype/pb"
)

func generateMnemonic(data []byte)  {
	var msg pb.GenerateMnemonic
	err := proto.Unmarshal(data, &msg)
	if err != nil {
		fmt.Printf("unmarshal err: %s\n",err.Error())
		CallbackError(err)
		return
	}
	fmt.Printf("generateMnemonic %d\n", msg.WordsCount)
	s, err := mobile.GenerateMnemonic(int(msg.WordsCount))
	if err != nil {
		CallbackError(err)
	}

	Callback("PrintMnemonic",  &pb.PrintMnemonic{Mnemonic: s})
}
