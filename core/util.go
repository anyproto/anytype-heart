package core

import (
	"math/rand"
	"time"

	"errors"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) getResponseEvent(ctx session.Context) *pb.ResponseEvent {
	ev := ctx.GetResponseEvent()
	mw.applicationService.GetEventSender().BroadcastToOtherSessions(ctx.ID(), &pb.Event{
		Messages:  ev.Messages,
		ContextId: ev.ContextId,
	})
	return ev
}

type errToCodeTuple[T ~int32] struct {
	err  error
	code T
}

func errToCode[T ~int32](err error, code T) errToCodeTuple[T] {
	return errToCodeTuple[T]{err, code}
}

func mapErrorCode[T ~int32](err error, mappings ...errToCodeTuple[T]) T {
	if err == nil {
		return 0
	}
	for _, m := range mappings {
		if errors.Is(err, m.err) {
			return m.code
		}
	}
	// Unknown error
	return 1
}

func getErrorDescription(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
