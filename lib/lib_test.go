package lib

import (
	"fmt"
	"testing"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"
)

func TestUnpack(t *testing.T) {
	b, _ := proto.Marshal(&pb.RpcWalletRecoverResponse{})

	var msg pb.RpcWalletRecoverResponse
	err := proto.Unmarshal(b, &msg)
	if err != nil {
		fmt.Println(err.Error())
	}
}

func TestEventHandler(t *testing.T) {
	var eventReceived *pb.Event
	mw = &core.Middleware{}
	SetEventHandler(func(event *pb.Event) {
		eventReceived = event
	})

	eventSent := &pb.Event{Message: &pb.EventMessageOfAccountShow{AccountShow: &pb.EventAccountShow{Index: 0, Account: &model.Account{Id: "1", Name: "name"}}}}
	mw.SendEvent(eventSent)

	require.Equal(t, eventSent, eventReceived, "eventReceived not equal to eventSent: %s %s", eventSent, eventReceived)
}
