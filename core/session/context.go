package session

import (
	"context"

	"github.com/anyproto/anytype-heart/pb"
)

type Context interface {
	ID() string
	Context() context.Context
	WithContext(context.Context) Context
	ObjectID() string
	SpaceID() string
	TraceID() string
	SetMessages(smartBlockId string, msgs []*pb.EventMessage)
	GetMessages() []*pb.EventMessage
	GetResponseEvent() *pb.ResponseEvent
}

type sessionContext struct {
	ctx          context.Context
	smartBlockId string
	spaceID      string
	traceId      string
	messages     []*pb.EventMessage
	sessionToken string
}

func NewContext(cctx context.Context, spaceID string, opts ...ContextOption) Context {
	if spaceID == "" {
		panic("spaceID is empty")
	}
	// TODO Add panic if spaceID is empty when working on the next step
	ctx := &sessionContext{
		spaceID: spaceID,
		ctx:     cctx,
	}
	for _, apply := range opts {
		apply(ctx)
	}
	return ctx
}

func (ctx *sessionContext) shallowCopy() *sessionContext {
	return &sessionContext{
		ctx:          ctx.ctx,
		spaceID:      ctx.spaceID,
		smartBlockId: ctx.smartBlockId,
		traceId:      ctx.traceId,
		messages:     ctx.messages,
		sessionToken: ctx.sessionToken,
	}
}

func (ctx *sessionContext) WithContext(cctx context.Context) Context {
	child := ctx.shallowCopy()
	child.ctx = cctx
	return child
}

// NewChildContext creates a new child context. The child context has empty messages
func NewChildContext(parent Context) Context {
	child := &sessionContext{
		ctx:          parent.Context(),
		spaceID:      parent.SpaceID(),
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

func (ctx *sessionContext) SpaceID() string {
	return ctx.spaceID
}

func (ctx *sessionContext) Context() context.Context {
	return ctx.ctx
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
