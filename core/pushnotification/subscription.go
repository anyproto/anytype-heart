package pushnotification

import (
	"encoding/base64"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type spaceViewStatus struct {
	spaceId        string
	spaceViewId    string
	topics         model.PushNotificationTopics
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
			{
				RelationKey: bundle.RelationKeySpaceAccountStatus,
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value: domain.Int64List(
					[]model.SpaceStatus{model.SpaceStatus_SpaceDeleted, model.SpaceStatus_SpaceRemoving},
				),
			},
		},
	}

	objectSubscription := objectsubscription.New[spaceViewStatus](service, objectsubscription.SubscriptionParams[spaceViewStatus]{
		Request: objectReq,
		Extract: func(details *domain.Details) (string, spaceViewStatus) {
			defer wakeUp()
			spaceKeyBase64 := details.GetString(bundle.RelationKeySpacePushNotificationsKey)
			spaceKey, _ := decodePrivKey(spaceKeyBase64)
			encKeyBase64 := details.GetString(bundle.RelationKeySpacePushNotificationsEncryptionKey)
			encKey, _ := decodeSymKey(encKeyBase64)
			return details.GetString(bundle.RelationKeyId), spaceViewStatus{
				spaceId:        details.GetString(bundle.RelationKeyTargetSpaceId),
				spaceViewId:    details.GetString(bundle.RelationKeyId),
				spaceKeyBase64: spaceKeyBase64,
				spaceKey:       spaceKey,
				encKeyBase64:   encKeyBase64,
				encKey:         encKey,
				topics:         model.PushNotificationTopics(details.GetInt64(bundle.RelationKeySpacePushNotificationsTopics)),
				creator:        details.GetString(bundle.RelationKeyCreator),
			}
		},
		Update: func(key string, value domain.Value, status spaceViewStatus) spaceViewStatus {
			defer wakeUp()
			switch domain.RelationKey(key) {
			case bundle.RelationKeySpacePushNotificationsKey:
				keyBase64 := value.String()
				if status.spaceKeyBase64 != keyBase64 {
					status.spaceKeyBase64 = keyBase64
					status.spaceKey, _ = decodePrivKey(keyBase64)
				}
			case bundle.RelationKeySpacePushNotificationsEncryptionKey:
				keyBase64 := value.String()
				if status.encKeyBase64 != keyBase64 {
					status.encKeyBase64 = keyBase64
					status.encKey, _ = decodeSymKey(keyBase64)
				}
			case bundle.RelationKeySpacePushNotificationsTopics:
				status.topics = model.PushNotificationTopics(value.Int64())
			case bundle.RelationKeyCreator:
				status.creator = value.String()
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

func decodePrivKey(keyBase64 string) (crypto.PrivKey, error) {
	keyMarshaled, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, err
	}
	return crypto.UnmarshalEd25519PrivateKeyProto(keyMarshaled)
}

func decodeSymKey(keyBase64 string) (crypto.SymKey, error) {
	keyMarshaled, err := base64.StdEncoding.DecodeString(keyBase64)
	if err != nil {
		return nil, err
	}
	return crypto.UnmarshallAESKeyProto(keyMarshaled)
}
