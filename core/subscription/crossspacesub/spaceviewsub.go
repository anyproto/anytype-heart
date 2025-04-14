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
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *service) runSpaceViewSub() error {
	resp, err := s.subscriptionService.Search(subscriptionservice.SubscribeRequest{
		SpaceId: s.spaceService.TechSpaceId(),
		Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyTargetSpaceId.String(), bundle.RelationKeySpaceLocalStatus.String()},
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
		s.handleSpaceViewDetails(r)
	}

	go s.monitorSpaceViewSub(resp.Output)

	return nil
}

func spaceIsAvailable(spaceViewDetails *domain.Details) bool {
	switch spaceViewDetails.GetInt64(bundle.RelationKeySpaceLocalStatus) {
	case int64(spaceinfo.LocalStatusUnknown), int64(spaceinfo.LocalStatusOk):
		return true
	default:
		return false
	}
}

func spaceIsDeleted(spaceViewDetails *domain.Details) bool {
	return spaceViewDetails.GetInt64(bundle.RelationKeySpaceLocalStatus) == int64(spaceinfo.LocalStatusMissing)
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

func (s *service) onSpaceViewSet(msg *pb.EventObjectDetailsSet) {
	details := domain.NewDetailsFromProto(msg.Details)
	s.spaceViewDetails[details.GetString(bundle.RelationKeyId)] = details

	s.handleSpaceViewDetails(details)
}

func (s *service) onSpaceViewAmend(msg *pb.EventObjectDetailsAmend) {
	details, ok := s.spaceViewDetails[msg.Id]
	if !ok {
		log.Error("amend space view: details not found", zap.String("id", msg.Id))
		return
	}
	for _, kv := range msg.Details {
		details.SetProtoValue(domain.RelationKey(kv.Key), kv.Value)
	}

	s.handleSpaceViewDetails(details)
}

func (s *service) onSpaceViewRemove(msg *pb.EventObjectSubscriptionRemove) {
	s.removeSpaceView(msg.Id)
}

func (s *service) handleSpaceViewDetails(details *domain.Details) {
	id := details.GetString(bundle.RelationKeyId)

	if spaceIsDeleted(details) {
		s.removeSpaceView(id)
	} else if spaceIsAvailable(details) {
		s.addSpaceView(details)
	}
}

func (s *service) addSpaceView(details *domain.Details) {
	id := details.GetString(bundle.RelationKeyId)

	if _, ok := s.spaceViewTargetIds[id]; !ok {
		targetId := details.GetString(bundle.RelationKeyTargetSpaceId)

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
