package event

import (
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pb"
)

const CName = "eventSender"

type Sender interface {
	IsActive(token string) bool
	Broadcast(event *pb.Event)
	BroadcastForSpace(spaceID string, event *pb.Event)
	SendToSession(token string, event *pb.Event)
	BroadcastToOtherSessions(token string, e *pb.Event)
	SetSpaceID(token string, spaceID string) error
	app.Component
}

type CallbackSender struct {
	callback func(event *pb.Event)
}

var _ = Sender(&CallbackSender{})

func (es *CallbackSender) Init(a *app.App) (err error) {
	return
}

func (es *CallbackSender) Name() (name string) {
	return CName
}

func NewCallbackSender(callback func(event *pb.Event)) *CallbackSender {
	return &CallbackSender{callback: callback}
}

func (es *CallbackSender) IsActive(_ string) bool {
	return true
}

func (es *CallbackSender) BroadcastToOtherSessions(token string, e *pb.Event) {
	// noop
}

func (es *CallbackSender) SendToSession(token string, event *pb.Event) {
	es.callback(event)
}

func (es *CallbackSender) Broadcast(event *pb.Event) {
	es.callback(event)
}

func (es *CallbackSender) BroadcastForSpace(_ string, event *pb.Event) {
	es.callback(event)
}

func (es *CallbackSender) SetSpaceID(_ string, _ string) error {
	// TODO think
	return nil
}
