package pushnotification

import (
	"encoding/base64"

	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type spaceViewStatus struct {
	spaceId        string
	spaceViewId    string
	mode           pb.RpcPushNotificationSetSpaceModeMode
	spaceKeyBase64 string
	spaceKey       crypto.PrivKey
	encKeyBase64   string
	encKey         crypto.SymKey
	creator        string
	muteIds        []string
	mentionIds     []string
	status         model.SpaceStatus
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
			bundle.RelationKeySpacePushNotificationKey.String(),
			bundle.RelationKeySpacePushNotificationEncryptionKey.String(),
			bundle.RelationKeySpacePushNotificationMode.String(),
			bundle.RelationKeySpaceAccountStatus.String(),
			bundle.RelationKeySpacePushNotificationCustomMuteIds.String(),
			bundle.RelationKeySpacePushNotificationCustomMentionIds.String(),
			bundle.RelationKeyCreator.String(),
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_spaceView)),
			},
			{
				RelationKey: bundle.RelationKeySpacePushNotificationKey,
				Condition:   model.BlockContentDataviewFilter_Exists,
			},
			{
				RelationKey: bundle.RelationKeyIsAclShared,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Bool(true),
			},
		},
	}

	objectSubscription := objectsubscription.New[spaceViewStatus](service,
		objectReq,
		objectsubscription.SubscriptionParams[spaceViewStatus]{
			SetDetails: func(details *domain.Details) (string, spaceViewStatus) {
				defer wakeUp()
				spaceKeyBase64 := details.GetString(bundle.RelationKeySpacePushNotificationKey)
				spaceKey, _ := decodePrivKey(spaceKeyBase64)
				encKeyBase64 := details.GetString(bundle.RelationKeySpacePushNotificationEncryptionKey)
				encKey, _ := decodeSymKey(encKeyBase64)
				return details.GetString(bundle.RelationKeyId), spaceViewStatus{
					spaceId:        details.GetString(bundle.RelationKeyTargetSpaceId),
					spaceViewId:    details.GetString(bundle.RelationKeyId),
					spaceKeyBase64: spaceKeyBase64,
					spaceKey:       spaceKey,
					encKeyBase64:   encKeyBase64,
					encKey:         encKey,
					mode:           pb.RpcPushNotificationSetSpaceModeMode(details.GetInt64(bundle.RelationKeySpacePushNotificationMode)),
					creator:        details.GetString(bundle.RelationKeyCreator),
				}
			},
			UpdateKeys: func(keyValues []objectsubscription.RelationKeyValue, status spaceViewStatus) spaceViewStatus {
				defer wakeUp()
				for _, kv := range keyValues {
					switch domain.RelationKey(kv.Key) {
					case bundle.RelationKeySpacePushNotificationKey:
						keyBase64 := kv.Value.String()
						if status.spaceKeyBase64 != keyBase64 {
							status.spaceKeyBase64 = keyBase64
							// nolint: errcheck
							status.spaceKey, _ = decodePrivKey(keyBase64)
						}
					case bundle.RelationKeySpacePushNotificationEncryptionKey:
						keyBase64 := kv.Value.String()
						if status.encKeyBase64 != keyBase64 {
							status.encKeyBase64 = keyBase64
							// nolint: errcheck
							status.encKey, _ = decodeSymKey(keyBase64)
						}
					case bundle.RelationKeySpacePushNotificationMode:
						// nolint: gosec
						status.mode = pb.RpcPushNotificationSetSpaceModeMode(kv.Value.Int64())
					case bundle.RelationKeySpacePushNotificationCustomMuteIds:
						status.muteIds = kv.Value.StringList()
					case bundle.RelationKeySpacePushNotificationCustomMentionIds:
						status.mentionIds = kv.Value.StringList()
					case bundle.RelationKeyCreator:
						status.creator = kv.Value.String()
					case bundle.RelationKeySpaceAccountStatus:
						// nolint: gosec
						status.status = model.SpaceStatus(kv.Value.Int64())
					}
				}
				return status
			},
			RemoveKeys: func(strings []string, status spaceViewStatus) spaceViewStatus {
				for _, key := range strings {
					if key == bundle.RelationKeySpacePushNotificationMode.String() {
						status.mode = pb.RpcPushNotificationSetSpaceMode_All
					} else if key == bundle.RelationKeySpacePushNotificationCustomMuteIds.String() {
						status.muteIds = nil
					} else if key == bundle.RelationKeySpacePushNotificationCustomMentionIds.String() {
						status.mentionIds = nil
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
	return crypto.UnmarshallAESKey(keyMarshaled)
}
