package core

import (
	"math/rand"
	"time"

	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) getResponseEvent(ctx session.Context) *pb.ResponseEvent {
	ev := ctx.GetResponseEvent()
	mw.EventSender.BroadcastToOtherSessions(ctx.ID(), &pb.Event{
		Messages:  ev.Messages,
		ContextId: ev.ContextId,
	})
	return ev
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
