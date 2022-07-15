package session

import "github.com/anytypeio/go-anytype-middleware/pb"

func NewContext(opts ...ContextOption) *Context {
	ctx := &Context{}
	for _, apply := range opts {
		apply(ctx)
	}
	return ctx
}

type ContextOption func(ctx *Context)

func WithSendEvent(se func(e *pb.Event)) ContextOption {
	return func(ctx *Context) {
		ctx.sendEvent = se
	}
}

func WithSessionId(sessionId string, sender SessionSender) ContextOption {
	return func(ctx *Context) {
		ctx.sessionId = sessionId
		ctx.sessionSender = sender
	}
}

func WithTraceId(traceId string) ContextOption {
	return func(ctx *Context) {
		ctx.traceId = traceId
	}
}

func NewChildContext(parent *Context) *Context {
	if parent == nil {
		return NewContext()
	}
	return &Context{
		smartBlockId:  parent.smartBlockId,
		traceId:       parent.traceId,
		sendEvent:     parent.sendEvent,
		sessionSender: parent.sessionSender,
		sessionId:     parent.sessionId,
	}
}

type SessionSender interface {
	SendSession(sessionId string, e *pb.Event)
}

type Context struct {
	smartBlockId  string
	traceId       string
	messages      []*pb.EventMessage
	sendEvent     func(e *pb.Event)
	sessionSender SessionSender
	sessionId     string
}

func (ctx *Context) AddMessages(smartBlockId string, msgs []*pb.EventMessage) {
	ctx.smartBlockId = smartBlockId
	ctx.messages = append(ctx.messages, msgs...)
	if ctx.sendEvent != nil {
		ctx.sendEvent(&pb.Event{
			Messages:  msgs,
			ContextId: smartBlockId,
			Initiator: nil,
		})
	}
}

func (ctx *Context) SetMessages(smartBlockId string, msgs []*pb.EventMessage) {
	ctx.smartBlockId = smartBlockId
	ctx.messages = msgs
	if ctx.sendEvent != nil {
		ctx.sendEvent(&pb.Event{
			Messages:  msgs,
			ContextId: smartBlockId,
			Initiator: nil,
		})
	}
}

func (ctx *Context) GetMessages() []*pb.EventMessage {
	return ctx.messages
}

func (ctx *Context) SendToOtherSessions(msgs []*pb.EventMessage) {
	if ctx.sessionSender != nil {
		ctx.sessionSender.SendSession(ctx.sessionId, &pb.Event{
			Messages:  msgs,
			ContextId: ctx.smartBlockId,
			Initiator: nil,
		})
	}
}

func (ctx *Context) GetResponseEvent() *pb.ResponseEvent {
	ctx.SendToOtherSessions(ctx.messages)
	return &pb.ResponseEvent{
		Messages:  ctx.messages,
		ContextId: ctx.smartBlockId,
		TraceId:   ctx.traceId,
	}
}
