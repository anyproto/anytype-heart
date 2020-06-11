package state

import "github.com/anytypeio/go-anytype-middleware/pb"

func NewContext(se func(e *pb.Event)) *Context {
	return &Context{sendEvent: se}
}

type Context struct {
	smartBlockId string
	messages     []*pb.EventMessage
	sendEvent    func(e *pb.Event)
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

func (ctx *Context) GetResponseEvent() *pb.ResponseEvent {
	return &pb.ResponseEvent{
		Messages:  ctx.messages,
		ContextId: ctx.smartBlockId,
	}
}
