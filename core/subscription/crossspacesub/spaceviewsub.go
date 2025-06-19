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
	"github.com/anyproto/anytype-heart/util/slice"
)

var deleteSpaceAccountStatuses = []model.SpaceStatus{model.SpaceStatus_SpaceDeleted, model.SpaceStatus_SpaceRemoving}

func (s *service) runSpaceViewSub() error {
	resp, err := s.subscriptionService.Search(subscriptionservice.SubscribeRequest{
		SpaceId: s.spaceService.TechSpaceId(),
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyTargetSpaceId.String(), bundle.RelationKeySpaceLocalStatus.String(), bundle.RelationKeySpaceAccountStatus.String(), bundle.RelationKeyCreator.String()},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				RelationKey: bundle.RelationKeySpaceAccountStatus,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value:       domain.Int64List(deleteSpaceAccountStatuses),
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
		s.spaceViewDetails[r.GetString(bundle.RelationKeyId)] = r
		s.processSpaceView(r)
	}

	go s.monitorSpaceViewSub(resp.Output)

	return nil
}

func (s *service) monitorSpaceViewSub(queue *mb.MB[*pb.EventMessage]) {
	matcher := subscriptionservice.EventMatcher{
		OnSet:    s.onSpaceViewSet,
		OnAmend:  s.onSpaceViewAmend,
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

func (s *service) onSpaceViewSet(techSpaceId string, msg *pb.EventObjectDetailsSet) {
	details := domain.NewDetailsFromProto(msg.Details)
	s.spaceViewDetails[details.GetString(bundle.RelationKeyId)] = details

	s.processSpaceView(details)
}

func (s *service) onSpaceViewAmend(techSpaceId string, msg *pb.EventObjectDetailsAmend) {
	details, ok := s.spaceViewDetails[msg.Id]
	if !ok {
		log.Error("amend space view: details not found", zap.String("id", msg.Id))
		return
	}
	for _, kv := range msg.Details {
		details.SetProtoValue(domain.RelationKey(kv.Key), kv.Value)
	}

	s.processSpaceView(details)
}

func (s *service) onSpaceViewRemove(techSpaceId string, msg *pb.EventObjectSubscriptionRemove) {
	s.removeSpaceView(msg.Id)
}

func (s *service) processSpaceView(details *domain.Details) {
	var (
		id       = details.GetString(bundle.RelationKeyId)
		targetId = details.GetString(bundle.RelationKeyTargetSpaceId)
	)

	if _, ok := s.spaceViewTargetIds[id]; !ok {
		s.spaceViewTargetIds[id] = targetId
		s.spaceIds = append(s.spaceIds, targetId)
	}

	for _, sub := range s.subscriptions {
		if sub.spacePredicate(details) {
			err := sub.AddSpace(targetId)
			if err != nil {
				log.Error("onSpaceViewSet: add space", zap.Error(err), zap.String("spaceId", targetId))
			}
		} else {
			sub.RemoveSpace(targetId)
		}
	}
}

func (s *service) removeSpaceView(spaceViewId string) {
	targetId, ok := s.spaceViewTargetIds[spaceViewId]
	if ok {
		for _, sub := range s.subscriptions {
			sub.RemoveSpace(targetId)
		}
		s.spaceIds = slice.RemoveMut(s.spaceIds, targetId)
		delete(s.spaceViewTargetIds, spaceViewId)
	}
}
