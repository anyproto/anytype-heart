package space

import (
	"fmt"
	"sync"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"go.uber.org/zap"
)

type spaceViewObjectSubscription = objectsubscription.ObjectSubscription[spaceViewStatus]

type spaceViewStatus struct {
	spaceId       string
	spaceViewId   string
	creator       string
	aclHeadId     string
	localStatus   spaceinfo.LocalStatus
	accountStatus spaceinfo.AccountStatus
	remoteStatus  spaceinfo.RemoteStatus
	guestKey      string
	mx            *sync.Mutex
}

func (s spaceViewStatus) String() string {
	return fmt.Sprintf("spaceViewStatus{spaceId: %s, spaceViewId: %s, creator: %s, aclHeadId: %s, localStatus: %s, accountStatus: %s, remoteStatus: %s, guestKey: %s}",
		s.spaceId, s.spaceViewId, s.creator, s.aclHeadId, s.localStatus, s.accountStatus, s.remoteStatus, s.guestKey)
}

type spaceSubscription struct {
	objSubscription *spaceViewObjectSubscription
	afterRun        func(sub *spaceViewObjectSubscription)
}

func newSpaceSubscription(
	service subscription.Service,
	techSpaceId string,
	afterRun func(sub *spaceViewObjectSubscription),
	update func(status spaceViewStatus),
) *spaceSubscription {
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
			bundle.RelationKeySpaceLocalStatus.String(),
			bundle.RelationKeyGuestKey.String(),
			bundle.RelationKeyLatestAclHeadId.String(),
		},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_spaceView)),
			},
		},
	}
	var objectSubscription *spaceViewObjectSubscription
	objectSubscription = objectsubscription.New(service, objectReq, objectsubscription.SubscriptionParams[spaceViewStatus]{
		SetDetails: func(details *domain.Details) (string, spaceViewStatus) {
			status := spaceViewStatus{
				spaceId:       details.GetString(bundle.RelationKeyTargetSpaceId),
				spaceViewId:   details.GetString(bundle.RelationKeyId),
				creator:       details.GetString(bundle.RelationKeyCreator),
				aclHeadId:     details.GetString(bundle.RelationKeyLatestAclHeadId),
				localStatus:   spaceinfo.LocalStatus(details.GetInt64(bundle.RelationKeySpaceLocalStatus)),
				accountStatus: spaceinfo.AccountStatus(details.GetInt64(bundle.RelationKeySpaceAccountStatus)),
				remoteStatus:  spaceinfo.RemoteStatus(details.GetInt64(bundle.RelationKeySpaceRemoteStatus)),
				guestKey:      details.GetString(bundle.RelationKeyGuestKey),
				mx:            &sync.Mutex{},
			}
			return details.GetString(bundle.RelationKeyId), status
		},
		UpdateKeys: func(keyValues []objectsubscription.RelationKeyValue, status spaceViewStatus) spaceViewStatus {
			for _, kv := range keyValues {
				switch domain.RelationKey(kv.Key) {
				case bundle.RelationKeyCreator:
					status.creator = kv.Value.String()
				case bundle.RelationKeySpaceRemoteStatus:
					status.remoteStatus = spaceinfo.RemoteStatus(kv.Value.Int64())
				case bundle.RelationKeySpaceAccountStatus:
					status.accountStatus = spaceinfo.AccountStatus(kv.Value.Int64())
				case bundle.RelationKeySpaceLocalStatus:
					status.localStatus = spaceinfo.LocalStatus(kv.Value.Int64())
				case bundle.RelationKeyLatestAclHeadId:
					status.aclHeadId = kv.Value.String()
				case bundle.RelationKeyGuestKey:
					status.aclHeadId = kv.Value.String()
				}
			}
			update(status)
			return status
		},
		RemoveKeys: func(strings []string, status spaceViewStatus) spaceViewStatus {
			// This should not be called for space views
			log.Error("remove keys for space view shouldn't be called", zap.Strings("keys", strings))
			return status
		},
		OnAdded: func(id string, entry spaceViewStatus) {
			update(entry)
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
