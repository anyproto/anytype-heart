package service

import (
	"fmt"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

	eventSent := &pb.Event{Messages: []*pb.EventMessage{{&pb.EventMessageValueOfAccountShow{AccountShow: &pb.EventAccountShow{Index: 0, Account: &model.Account{Id: "1", Name: "name"}}}}}}
	mw.EventSender.Broadcast(eventSent)

	require.Equal(t, eventSent, eventReceived, "eventReceived not equal to eventSent: %s %s", eventSent, eventReceived)
}
