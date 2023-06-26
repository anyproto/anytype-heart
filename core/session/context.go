package session

import "github.com/anyproto/anytype-heart/pb"

type Context struct {
	smartBlockId  string
	traceId       string
	messages      []*pb.EventMessage
	sessionSender Sender
	sessionToken  string
	isAsync       bool
}

func NewContext(opts ...ContextOption) *Context {
	ctx := &Context{}
	for _, apply := range opts {
		apply(ctx)
	}
	return ctx
}

func NewChildContext(parent *Context) *Context {
	if parent == nil {
		return NewContext()
	}
	return &Context{
		smartBlockId:  parent.smartBlockId,
		traceId:       parent.traceId,
		sessionSender: parent.sessionSender,
		sessionToken:  parent.sessionToken,
	}
}

func NewAsyncChildContext(parent *Context) *Context {
	ctx := NewChildContext(parent)
	ctx.isAsync = true
	return ctx
}

type ContextOption func(ctx *Context)

func Async() ContextOption {
	return func(ctx *Context) {
		ctx.isAsync = true
	}
}

func WithSession(token string, sender Sender) ContextOption {
	return func(ctx *Context) {
		ctx.sessionToken = token
		ctx.sessionSender = sender
	}
}

func WithTraceId(traceId string) ContextOption {
	return func(ctx *Context) {
		ctx.traceId = traceId
	}
}

type Sender interface {
	SendSession(token string, e *pb.Event)
}

type Closer interface {
	CloseSession(token string)
}

func (ctx *Context) AddMessages(smartBlockId string, msgs []*pb.EventMessage) {
	ctx.smartBlockId = smartBlockId
	ctx.messages = append(ctx.messages, msgs...)
}

func (ctx *Context) SetMessages(smartBlockId string, msgs []*pb.EventMessage) {
	ctx.smartBlockId = smartBlockId
	ctx.messages = msgs
}

func (ctx *Context) GetMessages() []*pb.EventMessage {
	return ctx.messages
}

func (ctx *Context) SendToOtherSessions(msgs []*pb.EventMessage) {
	if ctx.sessionSender != nil {
		ctx.sessionSender.SendSession(ctx.sessionToken, &pb.Event{
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
