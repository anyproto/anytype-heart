package spacev2

import (
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

// spaceViewEvent is a snapshot of a SpaceView at event time. The reconciler
// re-reads live state, so consumers treat this as a wakeup with routing info
// (spaceId), not as authoritative state — except for the remote-deletion
// reaction, which mirrors v1 in using the event snapshot.
type spaceViewEvent struct {
	spaceId       string
	viewId        string
	accountStatus spaceinfo.AccountStatus
	localStatus   spaceinfo.LocalStatus
	remoteStatus  spaceinfo.RemoteStatus
	aclHeadId     string
	guestKey      string
	creator       string
}

// spaceWatcher turns tech-space SpaceView changes into onEvent calls — the
// reactive spine driving controllers. No dedup queue is needed: onEvent must
// be non-blocking (ensure controller + poke), and the controller's buffered
// poke coalesces bursts per space.
type spaceWatcher struct {
	sub     *objectsubscription.ObjectSubscription[spaceViewEvent]
	onEvent func(ev spaceViewEvent)
}

func newSpaceWatcher(service subscription.Service, techSpaceId string, onEvent func(ev spaceViewEvent)) *spaceWatcher {
	req := subscription.SubscribeRequest{
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
	sub := objectsubscription.New(service, req, objectsubscription.SubscriptionParams[spaceViewEvent]{
		SetDetails: func(details *domain.Details) (string, spaceViewEvent) {
			ev := spaceViewEvent{
				spaceId:       details.GetString(bundle.RelationKeyTargetSpaceId),
				viewId:        details.GetString(bundle.RelationKeyId),
				creator:       details.GetString(bundle.RelationKeyCreator),
				aclHeadId:     details.GetString(bundle.RelationKeyLatestAclHeadId),
				localStatus:   spaceinfo.LocalStatus(details.GetInt64(bundle.RelationKeySpaceLocalStatus)),
				accountStatus: spaceinfo.AccountStatus(details.GetInt64(bundle.RelationKeySpaceAccountStatus)),
				remoteStatus:  spaceinfo.RemoteStatus(details.GetInt64(bundle.RelationKeySpaceRemoteStatus)),
				guestKey:      details.GetString(bundle.RelationKeyGuestKey),
			}
			return ev.viewId, ev
		},
		UpdateKeys: func(keyValues []objectsubscription.RelationKeyValue, ev spaceViewEvent) spaceViewEvent {
			for _, kv := range keyValues {
				switch domain.RelationKey(kv.Key) {
				case bundle.RelationKeyCreator:
					ev.creator = kv.Value.String()
				case bundle.RelationKeySpaceRemoteStatus:
					ev.remoteStatus = spaceinfo.RemoteStatus(kv.Value.Int64())
				case bundle.RelationKeySpaceAccountStatus:
					ev.accountStatus = spaceinfo.AccountStatus(kv.Value.Int64())
				case bundle.RelationKeySpaceLocalStatus:
					ev.localStatus = spaceinfo.LocalStatus(kv.Value.Int64())
				case bundle.RelationKeyLatestAclHeadId:
					ev.aclHeadId = kv.Value.String()
				case bundle.RelationKeyGuestKey:
					ev.guestKey = kv.Value.String()
				}
			}
			onEvent(ev)
			return ev
		},
		RemoveKeys: func(keys []string, ev spaceViewEvent) spaceViewEvent {
			log.Error("space view keys removed unexpectedly", zap.Strings("keys", keys))
			return ev
		},
		OnAdded: func(id string, ev spaceViewEvent) {
			onEvent(ev)
		},
	})
	return &spaceWatcher{sub: sub, onEvent: onEvent}
}

// Run starts the subscription and replays all pre-existing SpaceViews (the
// initial snapshot does not fire OnAdded) — this is how existing accounts
// discover their spaces on start.
func (w *spaceWatcher) Run() error {
	if err := w.sub.Run(); err != nil {
		return err
	}
	w.sub.Iterate(func(id string, ev spaceViewEvent) bool {
		w.onEvent(ev)
		return true
	})
	return nil
}

func (w *spaceWatcher) Close() {
	w.sub.Close()
}
