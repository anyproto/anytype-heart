package core

import (
	"errors"
	"math/rand"
	"time"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
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

func filtersFromProto(filters []*model.BlockContentDataviewFilter) []database.FilterRequest {
	var res []database.FilterRequest
	for _, f := range filters {
		res = append(res, database.FilterRequest{
			Id:               f.Id,
			Operator:         f.Operator,
			RelationKey:      domain.RelationKey(f.RelationKey),
			RelationProperty: f.RelationProperty,
			Condition:        f.Condition,
			Value:            domain.ValueFromProto(f.Value),
			QuickOption:      f.QuickOption,
			Format:           f.Format,
			IncludeTime:      f.IncludeTime,
		})
	}
	return res
}

func sortsFromProto(sorts []*model.BlockContentDataviewSort) []database.SortRequest {
	var res []database.SortRequest
	for _, s := range sorts {
		custom := make([]domain.Value, 0, len(s.CustomOrder))
		for _, item := range s.CustomOrder {
			custom = append(custom, domain.ValueFromProto(item))
		}
		res = append(res, database.SortRequest{
			RelationKey:    domain.RelationKey(s.RelationKey),
			Type:           s.Type,
			CustomOrder:    custom,
			Format:         s.Format,
			IncludeTime:    s.IncludeTime,
			Id:             s.Id,
			EmptyPlacement: s.EmptyPlacement,
		})
	}
	return res
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
