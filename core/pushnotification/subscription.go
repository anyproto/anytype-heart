package pushnotification

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type spaceViewStatus struct {
	spaceId     string
	spaceViewId string
	topics      model.PushNotificationTopics
	status      spaceinfo.LocalStatus
	shareable   bool
}

func newSpaceViewSubscription(service subscription.Service, techSpaceId string, wakeUp func()) (*objectsubscription.ObjectSubscription[spaceViewStatus], error) {
	objectReq := subscription.SubscribeRequest{
		SpaceId:           techSpaceId,
		SubId:             CName,
		Internal:          true,
		NoDepSubscription: true,
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyTargetSpaceId.String(),
			bundle.RelationKeySpaceLocalStatus.String(),
			bundle.RelationKeySpaceShareableStatus.String(),
			bundle.RelationKeyPushNotificationTopics.String(),
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyTargetSpaceId,
				Condition:   model.BlockContentDataviewFilter_Exists,
			},
			{
				RelationKey: bundle.RelationKeyTargetSpaceId,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.String(techSpaceId),
			},
		},
	}

	objectSubscription := objectsubscription.New[spaceViewStatus](service, objectsubscription.SubscriptionParams[spaceViewStatus]{
		Request: objectReq,
		Extract: func(details *domain.Details) (string, spaceViewStatus) {
			shareable := details.GetInt64(bundle.RelationKeySpaceShareableStatus) == int64(model.SpaceShareableStatus_StatusShareable)
			defer wakeUp()
			return details.GetString(bundle.RelationKeyId), spaceViewStatus{
				spaceId:     details.GetString(bundle.RelationKeyTargetSpaceId),
				spaceViewId: details.GetString(bundle.RelationKeyId),
				shareable:   shareable,
				topics:      model.PushNotificationTopics(details.GetInt64(bundle.RelationKeyPushNotificationTopics)),
				status:      spaceinfo.LocalStatus(details.GetInt64(bundle.RelationKeySpaceLocalStatus)),
			}
		},
		Update: func(key string, value domain.Value, status spaceViewStatus) spaceViewStatus {
			defer wakeUp()
			switch domain.RelationKey(key) {
			case bundle.RelationKeySpaceLocalStatus:
				status.status = spaceinfo.LocalStatus(value.Int64())
				return status
			case bundle.RelationKeySpaceShareableStatus:
				status.shareable = value.Int64() == int64(model.SpaceShareableStatus_StatusShareable)
				return status
			case bundle.RelationKeyPushNotificationTopics:
				status.topics = model.PushNotificationTopics(value.Int64())
				return status
			}
			return status
		},
		Unset: func(strings []string, status spaceViewStatus) spaceViewStatus {
			for _, key := range strings {
				if key == bundle.RelationKeyPushNotificationTopics.String() {
					status.topics = model.PushNotificationTopics_All
				}
			}
			return status
		},
	})
	if err := objectSubscription.Run(); err != nil {
		return nil, err
	}
	return objectSubscription, nil
}
