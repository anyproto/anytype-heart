package state

import "github.com/anytypeio/go-anytype-middleware/pb"

func NewContext(se func(e *pb.Event)) *Context {
	return &Context{sendEvent: se}
}

type Context struct {
	messages  []*pb.EventMessage
	sendEvent func(e *pb.Event)
}

func (ctx *Context) SetMessages(smartBlockId string, msgs []*pb.EventMessage) {
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
