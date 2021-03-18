package event

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

const CName = "eventSender"

type Sender interface {
	Send(event *pb.Event)
	app.Component
}

type CallbackSender struct {
	callback func(event *pb.Event)
}

func (es *CallbackSender) Init(a *app.App) (err error) {
	return
}

func (es *CallbackSender) Name() (name string) {
	return CName
}

func NewCallbackSender(callback func(event *pb.Event)) *CallbackSender {
	return &CallbackSender{callback: callback}
}

func (es *CallbackSender) Send(event *pb.Event) {
	es.callback(event)
}
