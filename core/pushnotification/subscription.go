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
	mode           pb.RpcPushNotificationMode
	spaceKeyBase64 string
	spaceKey       crypto.PrivKey
	encKeyBase64   string
	encKey         crypto.SymKey
	creator        string
	muteIds        []string
	mentionIds     []string
	allIds         []string
	status         model.SpaceStatus
	spaceType      model.SpaceType
}

type chatEntry struct {
	chatId       string
	spaceId      string
	isDiscussion bool
	subscribers  []string // participantIds from notificationSubscribers
}

func newChatSubscriptionParams(wakeUp func()) objectsubscription.SubscriptionParams[chatEntry] {
	return objectsubscription.SubscriptionParams[chatEntry]{
		SetDetails: func(details *domain.Details) (string, chatEntry) {
			defer wakeUp()
			id := details.GetString(bundle.RelationKeyId)
			return id, chatEntry{
				chatId:       id,
				spaceId:      details.GetString(bundle.RelationKeySpaceId),
				isDiscussion: isDiscussionLayout(details.GetInt64(bundle.RelationKeyResolvedLayout)),
				subscribers:  details.GetStringList(bundle.RelationKeyNotificationSubscribers),
			}
		},
		UpdateKeys: func(keyValues []objectsubscription.RelationKeyValue, entry chatEntry) chatEntry {
			defer wakeUp()
			for _, kv := range keyValues {
				switch domain.RelationKey(kv.Key) {
				case bundle.RelationKeyResolvedLayout:
					entry.isDiscussion = isDiscussionLayout(kv.Value.Int64())
				case bundle.RelationKeyNotificationSubscribers:
					entry.subscribers = kv.Value.StringList()
				case bundle.RelationKeySpaceId:
					entry.spaceId = kv.Value.String()
				}
			}
			return entry
		},
		RemoveKeys: func(keys []string, entry chatEntry) chatEntry {
			defer wakeUp()
			for _, key := range keys {
				if key == bundle.RelationKeyNotificationSubscribers.String() {
					entry.subscribers = nil
				}
			}
			return entry
		},
		OnAdded: func(string, chatEntry) {
			wakeUp()
		},
		OnRemoved: func(string, chatEntry) {
			wakeUp()
		},
	}
}

func isDiscussionLayout(layout int64) bool {
	// nolint: gosec
	return model.ObjectTypeLayout(layout) == model.ObjectType_discussion
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
			bundle.RelationKeySpacePushNotificationForceMuteIds.String(),
			bundle.RelationKeySpacePushNotificationForceMentionIds.String(),
			bundle.RelationKeySpacePushNotificationForceAllIds.String(),
			bundle.RelationKeySpaceType.String(),
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
					allIds:         details.GetStringList(bundle.RelationKeySpacePushNotificationForceAllIds),
					mentionIds:     details.GetStringList(bundle.RelationKeySpacePushNotificationForceMentionIds),
					muteIds:        details.GetStringList(bundle.RelationKeySpacePushNotificationForceMuteIds),
					// nolint: gosec
					mode:    pb.RpcPushNotificationMode(details.GetInt64(bundle.RelationKeySpacePushNotificationMode)),
					creator: details.GetString(bundle.RelationKeyCreator),
					// nolint: gosec
					status: model.SpaceStatus(details.GetInt64(bundle.RelationKeySpaceAccountStatus)),
					// nolint: gosec
					spaceType: model.SpaceType(details.GetInt64(bundle.RelationKeySpaceType)),
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
						status.mode = pb.RpcPushNotificationMode(kv.Value.Int64())
					case bundle.RelationKeySpacePushNotificationForceMuteIds:
						status.muteIds = kv.Value.StringList()
					case bundle.RelationKeySpacePushNotificationForceMentionIds:
						status.mentionIds = kv.Value.StringList()
					case bundle.RelationKeySpacePushNotificationForceAllIds:
						status.allIds = kv.Value.StringList()
					case bundle.RelationKeyCreator:
						status.creator = kv.Value.String()
					case bundle.RelationKeySpaceAccountStatus:
						// nolint: gosec
						status.status = model.SpaceStatus(kv.Value.Int64())
					case bundle.RelationKeySpaceType:
						// nolint: gosec
						status.spaceType = model.SpaceType(kv.Value.Int64())
					}
				}
				return status
			},
			RemoveKeys: func(strings []string, status spaceViewStatus) spaceViewStatus {
				for _, key := range strings {
					if key == bundle.RelationKeySpacePushNotificationMode.String() {
						status.mode = pb.RpcPushNotification_All
					} else if key == bundle.RelationKeySpacePushNotificationForceMuteIds.String() {
						status.muteIds = nil
					} else if key == bundle.RelationKeySpacePushNotificationForceMentionIds.String() {
						status.mentionIds = nil
					} else if key == bundle.RelationKeySpacePushNotificationForceAllIds.String() {
						status.allIds = nil
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
