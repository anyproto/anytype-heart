package syncstatus

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/anyproto/any-sync/nodeconf"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

type updateReceiver struct {
	coreService core.Service
	emitter     func(event *pb.Event)

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
	emitter func(event *pb.Event),
) *updateReceiver {
	if cfg.DisableThreadsSyncEvents {
		emitter = nil
	}
	return &updateReceiver{
		coreService:        coreService,
		linkedFilesWatcher: linkedFilesWatcher,
		subObjectsWatcher:  subObjectsWatcher,
		nodeConfService:    nodeConfService,
		emitter:            emitter,
		lastStatus:         make(map[string]pb.EventStatusThreadSyncStatus),
	}
}

func (r *updateReceiver) UpdateTree(ctx context.Context, objId string, status syncstatus.SyncStatus) (err error) {
	filesSummary := r.linkedFilesWatcher.GetLinkedFilesSummary(objId)
	objStatus := r.fetchObjectStatus(status)

	if lastObjStatus, found := r.lastStatus[objId]; found && objStatus == lastObjStatus && !filesSummary.isUpdated {
		return
	}
	r.lastStatus[objId] = objStatus

	r.notify(objId, objStatus, filesSummary.pinStatus)

	if objId == r.coreService.PredefinedBlocks().Account {
		r.subObjectsWatcher.ForEach(func(subObjectID string) {
			r.notify(subObjectID, objStatus, filesSummary.pinStatus)
		})
	}
	return
}

func (r *updateReceiver) fetchObjectStatus(status syncstatus.SyncStatus) pb.EventStatusThreadSyncStatus {
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
	if r.emitter == nil {
		return
	}
	r.emitter(&pb.Event{
		Messages:  []*pb.EventMessage{{Value: event}},
		ContextId: ctx,
	})
}
