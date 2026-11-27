package session

import (
	"context"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
)

type Context interface {
	ID() string
	ObjectID() string
	TraceID() string
	SetMessages(smartBlockId string, msgs []*pb.EventMessage)
	GetMessages() []*pb.EventMessage
	GetResponseEvent() *pb.ResponseEvent
	// IntegrationName is the RAW app name of the local-API session this
	// request came through, or "" for every other caller (the apps' gRPC
	// calls, internal work). It is how a request's attribution reaches the
	// states it applies: smartblock.Apply stamps the change of a state
	// created with NewStateCtx(ctx) with it (docs/ApiChangeAttribution.md).
	IntegrationName() string
}

type sessionContext struct {
	smartBlockId string
	traceId      string
	messages     []*pb.EventMessage
	sessionToken string
	// integrationName: see Context.IntegrationName. Per-request, like the
	// rest of this struct — a session context must never outlive its RPC.
	integrationName string
}

func NewContext(opts ...ContextOption) Context {
	ctx := &sessionContext{}
	for _, apply := range opts {
		apply(ctx)
	}
	return ctx
}

// NewChildContext creates a new child context. The child context has empty messages
func NewChildContext(parent Context) Context {
	child := &sessionContext{
		smartBlockId: parent.ObjectID(),
		traceId:      parent.TraceID(),
		sessionToken: parent.ID(),
		// a child context still serves the parent's request
		integrationName: parent.IntegrationName(),
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

// WithIntegrationName marks the context as serving a local-API request of
// the app with this RAW name (domain.IntegrationNameFromCtx of the request
// context). An empty name marks nothing.
func WithIntegrationName(name string) ContextOption {
	return func(ctx *sessionContext) {
		ctx.integrationName = name
	}
}

// attributionContext is a Context that carries a request's attribution and
// NOTHING else: it has no session, collects no events and answers no
// response. See AttributionOnly.
type attributionContext struct {
	integrationName string
}

// AttributionOnly returns a Context for an edit path whose state is built
// from a session.Context but which must keep running exactly as it does
// with a nil one — its events broadcast, nothing collected for a response —
// while its change is still attributed to the local-API request reqCtx
// belongs to. state.NewStateCtx recognizes it: the state it makes carries
// the app name as its explicit stamp and NO session context.
//
// It returns nil (an untyped nil Context) when reqCtx belongs to no API
// request, so a caller that passed nil before passes nil still.
func AttributionOnly(reqCtx context.Context) Context {
	name := domain.IntegrationNameFromCtx(reqCtx)
	if name == "" {
		return nil
	}
	return &attributionContext{integrationName: name}
}

// IsAttributionOnly reports whether ctx came from AttributionOnly.
func IsAttributionOnly(ctx Context) bool {
	_, ok := ctx.(*attributionContext)
	return ok
}

func (ctx *attributionContext) ID() string                             { return "" }
func (ctx *attributionContext) ObjectID() string                       { return "" }
func (ctx *attributionContext) TraceID() string                        { return "" }
func (ctx *attributionContext) SetMessages(string, []*pb.EventMessage) {}
func (ctx *attributionContext) GetMessages() []*pb.EventMessage        { return nil }
func (ctx *attributionContext) GetResponseEvent() *pb.ResponseEvent    { return &pb.ResponseEvent{} }
func (ctx *attributionContext) IntegrationName() string                { return ctx.integrationName }

type Closer interface {
	CloseSession(token string)
}

func (ctx *sessionContext) ID() string {
	return ctx.sessionToken
}

func (ctx *sessionContext) IntegrationName() string {
	return ctx.integrationName
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
