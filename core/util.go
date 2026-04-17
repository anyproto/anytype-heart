package core

import (
	"errors"
	"math/rand"
	"time"

	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

// universalBadInputErrors are errors that map to BAD_INPUT across any RPC response error enum.
// All generated Rpc*ResponseError_BAD_INPUT constants share the value 2, so we can safely type-assert.
const universalBadInputCode int32 = 2

var universalBadInputErrors = []error{
	idresolver.ErrEmptyObjectId,
}

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

	checkErrorType any
}

func errToCode[T ~int32](err error, code T) errToCodeTuple[T] {
	return errToCodeTuple[T]{err: err, code: code}
}

func errTypeToCode[T ~int32](errTypeProto any, code T) errToCodeTuple[T] {
	return errToCodeTuple[T]{code: code, checkErrorType: errTypeProto}
}

func mapErrorCode[T ~int32](err error, mappings ...errToCodeTuple[T]) T {
	if err == nil {
		return 0
	}
	for _, m := range mappings {
		if m.err != nil {
			if errors.Is(err, m.err) {
				return m.code
			}
		}
		if m.checkErrorType != nil {
			if errors.As(err, m.checkErrorType) {
				return m.code
			}
		}
	}
	for _, badInputErr := range universalBadInputErrors {
		if errors.Is(err, badInputErr) {
			return T(universalBadInputCode)
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
