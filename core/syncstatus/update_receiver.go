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
	lastStatus    map[string]pb.EventStatusThreadSyncStatus
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
		lastStatus:         make(map[string]pb.EventStatusThreadSyncStatus),
		eventSender:        eventSender,
	}
}

func (r *updateReceiver) UpdateTree(ctx context.Context, objId string, status syncstatus.SyncStatus) error {
	filesSummary := r.linkedFilesWatcher.GetLinkedFilesSummary(objId)
	objStatus := r.getObjectStatus(status)

	if !r.isStatusUpdated(objId, objStatus, filesSummary) {
		return nil
	}
	r.notify(objId, objStatus, filesSummary.pinStatus)

	// TODO For each space?
	if objId == r.coreService.PredefinedBlocks().Account {
		r.subObjectsWatcher.ForEach(func(subObjectID string) {
			r.notify(subObjectID, objStatus, filesSummary.pinStatus)
		})
	}
	return nil
}

func (r *updateReceiver) isStatusUpdated(objectID string, objStatus pb.EventStatusThreadSyncStatus, filesSummary linkedFilesSummary) bool {
	r.Lock()
	defer r.Unlock()
	if lastObjStatus, ok := r.lastStatus[objectID]; ok && objStatus == lastObjStatus && !filesSummary.isUpdated {
		return false
	}
	r.lastStatus[objectID] = objStatus
	return true
}

func (r *updateReceiver) getObjectStatus(status syncstatus.SyncStatus) pb.EventStatusThreadSyncStatus {
	if r.nodeConfService.NetworkCompatibilityStatus() == nodeconf.NetworkCompatibilityStatusIncompatible {
		return pb.EventStatusThread_IncompatibleVersion
	}

	if !r.isNodeConnected() {
		return pb.EventStatusThread_Offline
	}

	switch status {
	case syncstatus.StatusUnknown:
		return pb.EventStatusThread_Unknown
	case syncstatus.StatusSynced:
		return pb.EventStatusThread_Synced
	}
	return pb.EventStatusThread_Syncing
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
	objStatus pb.EventStatusThreadSyncStatus,
	pinStatus pb.EventStatusThreadCafePinStatus,
) {
	r.sendEvent(objId, &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
		Summary: &pb.EventStatusThreadSummary{Status: objStatus},
		Cafe: &pb.EventStatusThreadCafe{
			Status: objStatus,
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
