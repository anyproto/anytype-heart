package event

import "github.com/anytypeio/go-anytype-middleware/pb"

type Sender interface {
	Send(event *pb.Event)
}

type CallbackSender struct {
	callback func(event *pb.Event)
}

func NewCallbackSender(callback func(event *pb.Event)) *CallbackSender {
	return &CallbackSender{callback: callback}
}

func (es *CallbackSender) Send(event *pb.Event) {
	es.callback(event)
}
