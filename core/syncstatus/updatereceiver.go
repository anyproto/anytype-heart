package syncstatus

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/nodeconf"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/objectsyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

type updateReceiver struct {
	eventSender event.Sender

	nodeConfService nodeconf.Service
	lock            sync.Mutex
	nodeConnected   bool
	objectStore     objectstore.ObjectStore
	nodeStatus      nodestatus.NodeStatus
	spaceId         string
}

func newUpdateReceiver(
	nodeConfService nodeconf.Service,
	cfg *config.Config,
	eventSender event.Sender,
	objectStore objectstore.ObjectStore,
	nodeStatus nodestatus.NodeStatus,
) *updateReceiver {
	if cfg.DisableThreadsSyncEvents {
		eventSender = nil
	}
	return &updateReceiver{
		nodeConfService: nodeConfService,
		eventSender:     eventSender,
		objectStore:     objectStore,
		nodeStatus:      nodeStatus,
	}
}

func (r *updateReceiver) UpdateTree(_ context.Context, objId string, status objectsyncstatus.SyncStatus) error {
	objStatusEvent := r.getObjectSyncStatus(objId, status)
	r.notify(objId, objStatusEvent)
	return nil
}

func (r *updateReceiver) getFileStatus(fileId string) (filesyncstatus.Status, error) {
	details, err := r.objectStore.GetDetails(fileId)
	if err != nil {
		return filesyncstatus.Unknown, fmt.Errorf("get file details: %w", err)
	}
	if v, ok := details.GetDetails().GetFields()[bundle.RelationKeyFileBackupStatus.String()]; ok {
		return filesyncstatus.Status(v.GetNumberValue()), nil
	}
	return filesyncstatus.Unknown, fmt.Errorf("no backup status")
}

func (r *updateReceiver) getObjectSyncStatus(objectId string, status objectsyncstatus.SyncStatus) pb.EventStatusThreadSyncStatus {
	fileStatus, err := r.getFileStatus(objectId)
	if err == nil {
		// Prefer file backup status
		if fileStatus != filesyncstatus.Synced {
			status = fileStatus.ToSyncStatus()
		}
	}

	if r.nodeConfService.NetworkCompatibilityStatus() == nodeconf.NetworkCompatibilityStatusIncompatible {
		return pb.EventStatusThread_IncompatibleVersion
	}

	if !r.isNodeConnected() {
		return pb.EventStatusThread_Offline
	}

	switch status {
	case objectsyncstatus.StatusUnknown:
		return pb.EventStatusThread_Unknown
	case objectsyncstatus.StatusSynced:
		return pb.EventStatusThread_Synced
	}
	return pb.EventStatusThread_Syncing
}

func (r *updateReceiver) isNodeConnected() bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.nodeConnected
}

func (r *updateReceiver) setSpaceId(spaceId string) {
	r.lock.Lock()
	defer r.lock.Unlock()

	r.spaceId = spaceId
}

func (r *updateReceiver) UpdateNodeStatus() {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.nodeConnected = r.nodeStatus.GetNodeStatus(r.spaceId) == nodestatus.Online
}

func (r *updateReceiver) notify(
	objId string,
	objStatus pb.EventStatusThreadSyncStatus,
) {
	r.sendEvent(objId, &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
		Summary: &pb.EventStatusThreadSummary{Status: objStatus},
		Cafe: &pb.EventStatusThreadCafe{
			Status: objStatus,
			Files:  &pb.EventStatusThreadCafePinStatus{},
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
