package acl

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type spaceViewStatus struct {
	spaceId     string
	spaceViewId string
	creator     string
}

func newRemoveSelfSub(service subscription.Service, ownIdentity, techSpaceId string, wakeUp func()) (*objectsubscription.ObjectSubscription[spaceViewStatus], error) {
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

	objectSubscription := objectsubscription.New[spaceViewStatus](service, objectsubscription.SubscriptionParams[spaceViewStatus]{
		Request: objectReq,
		Extract: func(details *domain.Details) (string, spaceViewStatus) {
			defer wakeUp()
			return details.GetString(bundle.RelationKeyId), spaceViewStatus{
				spaceId:     details.GetString(bundle.RelationKeyTargetSpaceId),
				spaceViewId: details.GetString(bundle.RelationKeyId),
				creator:     details.GetString(bundle.RelationKeyCreator),
			}
		},
		Update: func(key string, value domain.Value, status spaceViewStatus) spaceViewStatus {
			defer wakeUp()
			switch domain.RelationKey(key) {
			case bundle.RelationKeyCreator:
				status.creator = value.String()
			}
			return status
		},
		Unset: func(strings []string, status spaceViewStatus) spaceViewStatus {
			defer wakeUp()
			return status
		},
	})
	if err := objectSubscription.Run(); err != nil {
		return nil, err
	}
	return objectSubscription, nil
}
