package session

import (
	"github.com/anyproto/anytype-heart/pb"
)

type Context interface {
	ID() string
	ObjectID() string
	TraceID() string
	SetMessages(smartBlockId string, msgs []*pb.EventMessage)
	GetMessages() []*pb.EventMessage
	GetResponseEvent() *pb.ResponseEvent
}

type sessionContext struct {
	smartBlockId string
	traceId      string
	messages     []*pb.EventMessage
	sessionToken string
}

func NewContext(opts ...ContextOption) Context {
	ctx := &sessionContext{}
	for _, apply := range opts {
		apply(ctx)
	}
	return ctx
}

func (ctx *sessionContext) shallowCopy() *sessionContext {
	return &sessionContext{
		smartBlockId: ctx.smartBlockId,
		traceId:      ctx.traceId,
		messages:     ctx.messages,
		sessionToken: ctx.sessionToken,
	}
}

// NewChildContext creates a new child context. The child context has empty messages
func NewChildContext(parent Context) Context {
	child := &sessionContext{
		smartBlockId: parent.ObjectID(),
		traceId:      parent.TraceID(),
		sessionToken: parent.ID(),
	}
	return child
}

type ContextOption func(ctx *sessionContext)

func WithSession(token string) ContextOption {
	return func(ctx *sessionContext) {
		ctx.sessionToken = token
	}
}

func WithTraceId(traceId string) ContextOption {
	return func(ctx *sessionContext) {
		ctx.traceId = traceId
	}
}

type Closer interface {
	CloseSession(token string)
}

func (ctx *sessionContext) ID() string {
	return ctx.sessionToken
}

func (ctx *sessionContext) ObjectID() string {
	return ctx.smartBlockId
}

func (ctx *sessionContext) TraceID() string {
	return ctx.traceId
}

func (ctx *sessionContext) AddMessages(smartBlockId string, msgs []*pb.EventMessage) {
	ctx.smartBlockId = smartBlockId
	ctx.messages = append(ctx.messages, msgs...)
}

func (ctx *sessionContext) SetMessages(smartBlockId string, msgs []*pb.EventMessage) {
	ctx.smartBlockId = smartBlockId
	ctx.messages = msgs
}

func (ctx *sessionContext) GetMessages() []*pb.EventMessage {
	return ctx.messages
}

func (ctx *sessionContext) GetResponseEvent() *pb.ResponseEvent {
	return &pb.ResponseEvent{
		Messages:  ctx.messages,
		ContextId: ctx.smartBlockId,
		TraceId:   ctx.traceId,
	}
}
