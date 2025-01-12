package core

import (
	"errors"
	"math/rand"
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
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
	return anyerror.CleanupError(err).Error()
}

func requestDetailsListToDomain(list []*model.Detail) []domain.Detail {
	details := make([]domain.Detail, 0, len(list))
	for _, it := range list {
		details = append(details, domain.Detail{
			Key:   domain.RelationKey(it.Key),
			Value: domain.ValueFromProto(it.Value),
		})
	}
	return details
}

func init() {
	rand.Seed(time.Now().UnixNano())
}
