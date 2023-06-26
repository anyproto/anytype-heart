package syncstatus

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/anyproto/any-sync/nodeconf"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

type updateReceiver struct {
	coreService core.Service
	eventSender event.Sender

	linkedFilesWatcher *linkedFilesWatcher
	subObjectsWatcher  *subObjectsWatcher
	nodeConfService    nodeconf.Service
	sync.Mutex
	nodeConnected bool
}

func newUpdateReceiver(
	coreService core.Service,
	linkedFilesWatcher *linkedFilesWatcher,
	subObjectsWatcher *subObjectsWatcher,
	nodeConfService nodeconf.Service,
	cfg *config.Config,
	eventSender event.Sender,
) *updateReceiver {
	if cfg.DisableThreadsSyncEvents {
		eventSender = nil
	}
	return &updateReceiver{
		coreService:        coreService,
		linkedFilesWatcher: linkedFilesWatcher,
		subObjectsWatcher:  subObjectsWatcher,
		nodeConfService:    nodeConfService,
		eventSender:        eventSender,
	}
}

func (r *updateReceiver) UpdateTree(ctx context.Context, objId string, status syncstatus.SyncStatus) (err error) {
	var (
		nodeConnected bool
		objStatus     pb.EventStatusThreadSyncStatus
		generalStatus pb.EventStatusThreadSyncStatus
	)

	nodeConnected = r.isNodeConnected()
	linkedFilesSummary := r.linkedFilesWatcher.GetLinkedFilesSummary(objId)

	networkStatus := r.nodeConfService.NetworkCompatibilityStatus()
	switch status {
	case syncstatus.StatusUnknown:
		objStatus = pb.EventStatusThread_Unknown
	case syncstatus.StatusSynced:
		objStatus = pb.EventStatusThread_Synced
	case syncstatus.StatusNotSynced:
		objStatus = pb.EventStatusThread_Syncing
	}

	switch networkStatus {
	case nodeconf.NetworkCompatibilityStatusIncompatible:
		objStatus = pb.EventStatusThread_IncompatibleVersion
	default:
		if !nodeConnected {
			objStatus = pb.EventStatusThread_Offline
		}
	}
	generalStatus = objStatus

	r.notify(objId, objStatus, generalStatus, linkedFilesSummary)

	if objId == r.coreService.PredefinedBlocks().Account {
		r.subObjectsWatcher.ForEach(func(subObjectID string) {
			r.notify(subObjectID, objStatus, generalStatus, linkedFilesSummary)
		})
	}
	return
}

func (r *updateReceiver) isNodeConnected() bool {
	r.Lock()
	defer r.Unlock()
	return r.nodeConnected
}

func (r *updateReceiver) UpdateNodeConnection(online bool) {
	r.Lock()
	defer r.Unlock()
	r.nodeConnected = online
}

func (r *updateReceiver) notify(
	objId string,
	objStatus, generalStatus pb.EventStatusThreadSyncStatus,
	pinStatus pb.EventStatusThreadCafePinStatus,
) {
	r.sendEvent(objId, &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
		Summary: &pb.EventStatusThreadSummary{Status: objStatus},
		Cafe: &pb.EventStatusThreadCafe{
			Status: generalStatus,
			Files:  &pinStatus,
		},
	}})
}

func (r *updateReceiver) sendEvent(ctx string, event pb.IsEventMessageValue) {
	if r.eventSender == nil {
		return
	}
	r.eventSender.Broadcast(&pb.Event{
		Messages:  []*pb.EventMessage{{Value: event}},
		ContextId: ctx,
	})
}
