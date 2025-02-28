package crossspacesub

import (
	"context"
	"errors"
	"fmt"

	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *service) runSpaceViewSub() error {
	resp, err := s.subscriptionService.Search(subscriptionservice.SubscribeRequest{
		SpaceId: s.spaceService.TechSpaceId(),
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyTargetSpaceId.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				RelationKey: bundle.RelationKeySpaceAccountStatus,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       domain.Int64List([]model.AccountStatusType{model.Account_Deleted}),
			},
			{
				RelationKey: bundle.RelationKeySpaceLocalStatus,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List([]model.SpaceStatus{model.SpaceStatus_Ok, model.SpaceStatus_Unknown}),
			},
		},
		Internal: true,
	})
	if err != nil {
		return fmt.Errorf("subscribe: %w", err)
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	s.spaceViewsSubId = resp.SubId
	s.spaceViewTargetIds = make(map[string]string, len(resp.Records))
	for _, r := range resp.Records {
		id := r.GetString(bundle.RelationKeyId)
		targetId := r.GetString(bundle.RelationKeyTargetSpaceId)
		s.spaceViewTargetIds[id] = targetId
		s.spaceIds = append(s.spaceIds, targetId)
	}

	go s.monitorSpaceViewSub(resp.Output)

	return nil
}

func (s *service) monitorSpaceViewSub(queue *mb.MB[*pb.EventMessage]) {
	matcher := subscriptionservice.EventMatcher{
		OnSet:    s.onSpaceViewSet,
		OnRemove: s.onSpaceViewRemove,
	}

	for {
		msgs, err := queue.Wait(s.componentCtx)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			log.Error("monitor space views", zap.Error(err))
			continue
		}

		s.lock.Lock()
		for _, msg := range msgs {
			matcher.Match(msg)
		}
		s.lock.Unlock()
	}
}

func (s *service) onSpaceViewSet(msg *pb.EventObjectDetailsSet) {
	id := pbtypes.GetString(msg.Details, bundle.RelationKeyId.String())
	targetId := pbtypes.GetString(msg.Details, bundle.RelationKeyTargetSpaceId.String())

	if _, ok := s.spaceViewTargetIds[id]; !ok {
		s.spaceViewTargetIds[id] = targetId
		s.spaceIds = append(s.spaceIds, targetId)

		for _, sub := range s.subscriptions {
			err := sub.AddSpace(targetId)
			if err != nil {
				log.Error("onSpaceViewSet: add space", zap.Error(err), zap.String("spaceId", targetId))
			}
		}
	}
}

func (s *service) onSpaceViewRemove(msg *pb.EventObjectSubscriptionRemove) {
	targetId, ok := s.spaceViewTargetIds[msg.Id]
	if ok {
		for _, sub := range s.subscriptions {
			sub.RemoveSpace(targetId)
		}
		s.spaceIds = slice.RemoveMut(s.spaceIds, targetId)
		delete(s.spaceViewTargetIds, msg.Id)
	}
}
