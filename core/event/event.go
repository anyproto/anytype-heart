package event

/*
AI generated

Name: Client Event Broadcaster
Scope: global

## Responsibility
- Broadcast pb.Event messages to connected client sessions
- Track active sessions and route events (to one, all, or filtered sessions)

## Background Tasks
- Session cleanup: handles session disconnection via shutdown channel (GrpcSender only)
*/

import (
	"context"
	"time"

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

// SessionWaiter is the optional half of Sender implemented by senders that route
// to client-owned streams (GrpcSender). Deliberately not part of Sender:
// CallbackSender hands events straight to the mobile library's callback, with no
// stream that could be late, so it must keep working untouched. Callers
// type-assert and skip the wait when it is not implemented.
type SessionWaiter interface {
	WaitForSession(ctx context.Context, token string, timeout time.Duration) bool
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

func NewMessage(spaceId string, value pb.IsEventMessageValue) *pb.EventMessage {
	return &pb.EventMessage{
		SpaceId: spaceId,
		Value:   value,
	}
}

func NewEventSingleMessage(spaceId string, value pb.IsEventMessageValue) *pb.Event {
	return &pb.Event{
		Messages: []*pb.EventMessage{NewMessage(spaceId, value)},
	}
}
