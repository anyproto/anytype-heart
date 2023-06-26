package session

import (
	"context"

	"github.com/anyproto/anytype-heart/pb"
)

type TestContext struct {
	context context.Context
}

func NewTestContext() Context {
	return &TestContext{}
}

func (c *TestContext) ID() string {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) SpaceID() string {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) ObjectID() string {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) TraceID() string {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) SetIsAsync(b bool) {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) Context() context.Context {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) IsActive() bool {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) SetMessages(smartBlockId string, msgs []*pb.EventMessage) {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) Send(event *pb.Event) {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) SendToOtherSessions(msgs []*pb.EventMessage) {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) GetMessages() []*pb.EventMessage {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) Broadcast(event *pb.Event) {
	// TODO implement me
	panic("implement me")
}

func (c *TestContext) GetResponseEvent() *pb.ResponseEvent {
	// TODO implement me
	panic("implement me")
}
