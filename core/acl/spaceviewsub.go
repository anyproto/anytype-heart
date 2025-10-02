package acl

import (
	"fmt"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type spaceViewObjectSubscription = objectsubscription.ObjectSubscription[spaceViewStatus]

type spaceViewStatus struct {
	spaceId     string
	spaceViewId string
	creator     string
}

func (s spaceViewStatus) String() string {
	return fmt.Sprintf("spaceId: %s, spaceViewId: %s, creator: %s", s.spaceId, s.spaceViewId, s.creator)
}

type spaceSubscription struct {
	objSubscription *spaceViewObjectSubscription
	afterRun        func(sub *spaceViewObjectSubscription)
}

func newSpaceSubscription(
	service subscription.Service,
	ownIdentity string,
	techSpaceId string,
	afterRun func(sub *spaceViewObjectSubscription),
	add func(status spaceViewStatus),
	remove func(id string, status spaceViewStatus),
) *spaceSubscription {
	participantId := domain.NewParticipantId(techSpaceId, ownIdentity)
	objectReq := subscription.SubscribeRequest{
		SpaceId:           techSpaceId,
		SubId:             CName,
		Internal:          true,
		NoDepSubscription: true,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyTargetSpaceId.String(),
			bundle.RelationKeyCreator.String(),
			bundle.RelationKeySpaceRemoteStatus.String(),
			bundle.RelationKeySpaceAccountStatus.String(),
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyCreator,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.String(participantId),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				RelationKey: bundle.RelationKeySpaceRemoteStatus,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.SpaceStatus_Ok)),
			},
			{
				RelationKey: bundle.RelationKeySpaceAccountStatus,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.SpaceStatus_SpaceDeleted)),
			},
			{
				RelationKey: bundle.RelationKeyMyParticipantStatus,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value: domain.Int64List(
					[]model.ParticipantStatus{model.ParticipantStatus_Removed, model.ParticipantStatus_Removing},
				),
			},
		},
	}
	objectSubscription := objectsubscription.New[spaceViewStatus](service, objectReq, objectsubscription.SubscriptionParams[spaceViewStatus]{
		SetDetails: func(details *domain.Details) (string, spaceViewStatus) {
			status := spaceViewStatus{
				spaceId:     details.GetString(bundle.RelationKeyTargetSpaceId),
				spaceViewId: details.GetString(bundle.RelationKeyId),
				creator:     details.GetString(bundle.RelationKeyCreator),
			}
			return details.GetString(bundle.RelationKeyId), status
		},
		UpdateKeys: func(keyValues []objectsubscription.RelationKeyValue, status spaceViewStatus) spaceViewStatus {
			for _, kv := range keyValues {
				if domain.RelationKey(kv.Key) == bundle.RelationKeyCreator {
					status.creator = kv.Value.String()
				}
			}
			return status
		},
		RemoveKeys: func(strings []string, status spaceViewStatus) spaceViewStatus {
			// This should not be called for space views
			log.Error("remove keys for space view shouldn't be called", zap.Strings("keys", strings))
			return status
		},
		OnAdded: func(id string, entry spaceViewStatus) {
			add(entry)
		},
		OnRemoved: func(id string, entry spaceViewStatus) {
			remove(id, entry)
		},
	})
	return &spaceSubscription{
		objSubscription: objectSubscription,
		afterRun:        afterRun,
	}
}

func (s *spaceSubscription) Run() error {
	defer s.afterRun(s.objSubscription)
	return s.objSubscription.Run()
}

func (s *spaceSubscription) Close() {
	s.objSubscription.Close()
}
