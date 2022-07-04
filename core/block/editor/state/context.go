package state

import "github.com/anytypeio/go-anytype-middleware/pb"

func NewContext(se func(e *pb.Event)) *Context {
	return &Context{sendEvent: se}
}

func NewChildContext(parent *Context) *Context {
	if parent == nil {
		return NewContext(nil)
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

func NewSessionContext(sessionId string, sessionSender SessionSender, se func(e *pb.Event)) *Context {
	return &Context{sendEvent: se, sessionId: sessionId, sessionSender: sessionSender}
}

func NewContextTrace(traceId string, se func(e *pb.Event)) *Context {
	return &Context{sendEvent: se, traceId: traceId}
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
