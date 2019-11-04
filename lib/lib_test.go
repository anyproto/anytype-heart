package lib

import (
	"fmt"
	"testing"

	"github.com/anytypeio/go-anytype-middleware/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
)

func Test_Unpack(t *testing.T) {
	b, _ := proto.Marshal(&pb.Rpc_Wallet_Recover_Response{})

	var msg pb.Rpc_Wallet_Recover_Response
	err := proto.Unmarshal(b, &msg)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func Test_EventHandler(t *testing.T) {
	var eventReceived *pb.Event
	mw = &core.Middleware{}
	SetEventHandler(func(event *pb.Event) {
		eventReceived = event
	})

	eventSent := &pb.Event{Message: &pb.Event_AccountShow{AccountShow: &pb.Event_Account_Show{Index: 0, Account: &pb.Model_Account{Id: "1", Name: "name"}}}}
	mw.SendEvent(eventSent)

	require.Equal(t, eventSent, eventReceived, "eventReceived not equal to eventSent: %s %s", eventSent, eventReceived)
}
