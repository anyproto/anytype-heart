package testapp

import (
	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func New() *TestApp {
	return &TestApp{&app.App{}}
}

type TestApp struct {
	*app.App
}

func (ta *TestApp) With(cmp app.Component) *TestApp {
	ta.Register(cmp)
	return ta
}

type EventSender struct {
	F func(e *pb.Event)
}

func (e *EventSender) Init(a *app.App) (err error) {
	return
}

func (e *EventSender) Name() (name string) {
	return event.CName
}

func (e *EventSender) Send(event *pb.Event) {
	if e.F != nil {
		e.F(event)
	}
}
