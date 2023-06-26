package session

import (
	"context"

	"github.com/anyproto/anytype-heart/pb"
)

type Context struct {
	ctx           context.Context
	smartBlockId  string
	spaceID       string
	traceId       string
	messages      []*pb.EventMessage
	sessionSender Sender
	sessionToken  string
	isAsync       bool
}

func NewContext(cctx context.Context, spaceID string, opts ...ContextOption) *Context {
	ctx := &Context{
		spaceID: spaceID,
	}
	for _, apply := range opts {
		apply(ctx)
	}
	return ctx
}

func NewChildContext(parent *Context) *Context {
	return &Context{
		ctx:           parent.ctx,
		spaceID:       parent.spaceID,
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
	IsActive(token string) bool
	SendToSession(token string, event *pb.Event)
	BroadcastForSpace(spaceID string, event *pb.Event)
	BroadcastToOtherSessions(token string, e *pb.Event)
}

type Closer interface {
	CloseSession(token string)
}

func (ctx *Context) ID() string {
	return ctx.sessionToken
}

func (ctx *Context) SpaceID() string {
	return ctx.spaceID
}

func (ctx *Context) IsAsync() bool {
	return ctx.isAsync
}

func (ctx *Context) Context() context.Context {
	return ctx.ctx
}

func (ctx *Context) IsActive() bool {
	// TODO Carefully check this. When session sender is nil?
	if ctx.sessionSender == nil {
		return false
	}
	return ctx.sessionSender.IsActive(ctx.sessionToken)
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

func (ctx *Context) Send(event *pb.Event) {
	ctx.sessionSender.SendToSession(ctx.sessionToken, event)
}

func (ctx *Context) Broadcast(event *pb.Event) {
	ctx.sessionSender.BroadcastForSpace(ctx.spaceID, event)
}

func (ctx *Context) SendToOtherSessions(msgs []*pb.EventMessage) {
	if ctx.sessionSender != nil {
		ctx.sessionSender.BroadcastToOtherSessions(ctx.sessionToken, &pb.Event{
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
