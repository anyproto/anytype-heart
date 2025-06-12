package pushnotification

import (
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type spaceViewStatus struct {
	spaceId        string
	spaceViewId    string
	topics         model.PushNotificationTopics
	status         spaceinfo.AccountStatus
	spaceKeyBase64 string
	spaceKey       crypto.PrivKey
	encKeyBase64   string
	encKey         crypto.SymKey
	creator        string
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
			bundle.RelationKeySpacePushNotificationsKey.String(),
			bundle.RelationKeySpacePushNotificationsEncryptionKey.String(),
			bundle.RelationKeySpacePushNotificationsTopics.String(),
			bundle.RelationKeySpaceAccountStatus.String(),
			bundle.RelationKeyCreator.String(),
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				RelationKey: bundle.RelationKeySpacePushNotificationsKey,
				Condition:   model.BlockContentDataviewFilter_Exists,
			},
		},
	}

	objectSubscription := objectsubscription.New[spaceViewStatus](service, objectsubscription.SubscriptionParams[spaceViewStatus]{
		Request: objectReq,
		Extract: func(details *domain.Details) (string, spaceViewStatus) {
			defer wakeUp()
			// TODO: extract keys
			return details.GetString(bundle.RelationKeyId), spaceViewStatus{
				spaceId:        details.GetString(bundle.RelationKeyTargetSpaceId),
				spaceViewId:    details.GetString(bundle.RelationKeyId),
				spaceKeyBase64: details.GetString(bundle.RelationKeySpacePushNotificationsKey),
				encKeyBase64:   details.GetString(bundle.RelationKeySpacePushNotificationsEncryptionKey),
				topics:         model.PushNotificationTopics(details.GetInt64(bundle.RelationKeySpacePushNotificationsTopics)),
				status:         spaceinfo.AccountStatus(details.GetInt64(bundle.RelationKeySpaceAccountStatus)),
			}
		},
		Update: func(key string, value domain.Value, status spaceViewStatus) spaceViewStatus {
			defer wakeUp()
			switch domain.RelationKey(key) {
			case bundle.RelationKeySpaceAccountStatus:
				status.status = spaceinfo.AccountStatus(value.Int64())
				return status
			}
			return status
		},
		Unset: func(strings []string, status spaceViewStatus) spaceViewStatus {
			for _, key := range strings {
				if key == bundle.RelationKeySpacePushNotificationsTopics.String() {
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
