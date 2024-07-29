package event

import (
	"sync"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/pb"
)

const CName = "eventSender"

type Sender interface {
	IsActive(token string) bool
	Broadcast(event *pb.Event)
	SendToSession(token string, event *pb.Event)
	BroadcastToOtherSessions(token string, e *pb.Event)
	Flush()
	app.Component
}

type CallbackSender struct {
	flushMx  sync.Mutex
	flushed  bool
	events   []*pb.Event
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

func (es *CallbackSender) Flush() {
	es.flushMx.Lock()
	if es.flushed {
		es.flushMx.Unlock()
		return
	}
	es.flushed = true
	es.flushMx.Unlock()
	// we can read from events without lock
	for _, event := range es.events {
		es.callback(event)
	}
	es.events = nil
}

func (es *CallbackSender) sendEvent(event *pb.Event) {
	es.flushMx.Lock()
	if !es.flushed {
		defer es.flushMx.Unlock()
		es.events = append(es.events, event)
		return
	}
	es.flushMx.Unlock()
	es.callback(event)
}
