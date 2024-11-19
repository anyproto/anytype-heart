package event

import (
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pb"
)

const CName = "eventSender"

type Sender interface {
	IsActive(token string) bool
	Broadcast(event *pb.Event)
	SendToSession(token string, event *pb.Event)
	BroadcastToOtherSessions(token string, e *pb.Event)
	BroadcastExceptSessions(event *pb.Event, exceptTokens []string)

	app.Component
}

type CallbackSender struct {
	callback func(event *pb.Event)
}

func NewCallbackSender(callback func(event *pb.Event)) *CallbackSender {
	return &CallbackSender{callback: callback}
}

var _ = Sender(&CallbackSender{})

func (es *CallbackSender) Init(a *app.App) (err error) {
	return
}

func (es *CallbackSender) Name() (name string) {
	return CName
}

func (es *CallbackSender) IsActive(token string) bool {
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

func (es *CallbackSender) BroadcastExceptSessions(event *pb.Event, exceptTokens []string) {
	es.callback(event)
}
