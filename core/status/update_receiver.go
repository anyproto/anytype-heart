package status

import (
	"context"
	"sync"

	"github.com/anytypeio/any-sync/commonspace/syncstatus"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
)

type UpdateReceiver struct {
	coreService core.Service
	emitter     func(event *pb.Event)

	linkedFilesWatcher LinkedFilesWatcher
	subObjectsWatcher  SubObjectsWatcher

	sync.Mutex
	nodeConnected bool
}

func NewUpdateReceiver(
	coreService core.Service,
	linkedFilesWatcher LinkedFilesWatcher,
	subObjectsWatcher SubObjectsWatcher,
	cfg *config.Config,
	emitter func(event *pb.Event),
) *UpdateReceiver {
	if cfg.DisableThreadsSyncEvents {
		emitter = nil
	}
	return &UpdateReceiver{
		coreService:        coreService,
		linkedFilesWatcher: linkedFilesWatcher,
		subObjectsWatcher:  subObjectsWatcher,
		emitter:            emitter,
	}
}

func (r *UpdateReceiver) UpdateTree(ctx context.Context, objId string, status syncstatus.SyncStatus) (err error) {
	var (
		nodeConnected bool
		objStatus     pb.EventStatusThreadSyncStatus
		generalStatus pb.EventStatusThreadSyncStatus
	)

	nodeConnected = r.isNodeConnected()
	linkedFilesSummary := r.linkedFilesWatcher.GetLinkedFilesSummary(objId)

	switch status {
	case syncstatus.StatusUnknown:
		objStatus = pb.EventStatusThread_Unknown
	case syncstatus.StatusSynced:
		objStatus = pb.EventStatusThread_Synced
	case syncstatus.StatusNotSynced:
		objStatus = pb.EventStatusThread_Syncing
	}
	if !nodeConnected {
		objStatus = pb.EventStatusThread_Offline
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

func (r *UpdateReceiver) isNodeConnected() bool {
	r.Lock()
	defer r.Unlock()
	return r.nodeConnected
}

func (r *UpdateReceiver) UpdateNodeConnection(online bool) {
	r.Lock()
	defer r.Unlock()
	r.nodeConnected = online
}

func (r *UpdateReceiver) notify(
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

func (r *UpdateReceiver) sendEvent(ctx string, event pb.IsEventMessageValue) {
	if r.emitter == nil {
		return
	}
	r.emitter(&pb.Event{
		Messages:  []*pb.EventMessage{{Value: event}},
		ContextId: ctx,
	})
}
