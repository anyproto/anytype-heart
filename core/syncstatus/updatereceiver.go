package syncstatus

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/dgraph-io/badger/v3"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

type updateReceiver struct {
	eventSender event.Sender

	nodeConfService nodeconf.Service
	sync.Mutex
	nodeConnected bool
	lastStatus    map[string]pb.EventStatusThreadSyncStatus
	badger        *badger.DB
}

func newUpdateReceiver(nodeConfService nodeconf.Service, cfg *config.Config, eventSender event.Sender, badger *badger.DB) *updateReceiver {
	if cfg.DisableThreadsSyncEvents {
		eventSender = nil
	}
	return &updateReceiver{
		nodeConfService: nodeConfService,
		lastStatus:      make(map[string]pb.EventStatusThreadSyncStatus),
		eventSender:     eventSender,
		badger:          badger,
	}
}

func (r *updateReceiver) UpdateTree(_ context.Context, objId string, status syncstatus.SyncStatus) error {
	objStatus := r.getObjectStatus(objId, status)

	if !r.isStatusUpdated(objId, objStatus) {
		return nil
	}
	r.notify(objId, objStatus)

	return nil
}

func (r *updateReceiver) isStatusUpdated(objectID string, objStatus pb.EventStatusThreadSyncStatus) bool {
	r.Lock()
	defer r.Unlock()
	if lastObjStatus, ok := r.lastStatus[objectID]; ok && objStatus == lastObjStatus {
		return false
	}
	r.lastStatus[objectID] = objStatus
	return true
}

func (r *updateReceiver) getFileStatus(fileId string) (FileStatus, error) {
	rawStatus, err := badgerhelper.GetValue(r.badger, []byte(fileStatusPrefix+fileId), badgerhelper.UnmarshalInt)
	if err != nil {
		return FileStatusUnknown, fmt.Errorf("get file status: %w", err)
	}
	return FileStatus(rawStatus), nil
}

func (r *updateReceiver) getObjectStatus(objectId string, status syncstatus.SyncStatus) pb.EventStatusThreadSyncStatus {
	fileStatus, err := r.getFileStatus(objectId)
	if err == nil {
		// Prefer file backup status
		if fileStatus != FileStatusSynced {
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
	case syncstatus.StatusUnknown:
		return pb.EventStatusThread_Unknown
	case syncstatus.StatusSynced:
		return pb.EventStatusThread_Synced
	}
	return pb.EventStatusThread_Syncing
}

func (r *updateReceiver) ClearLastObjectStatus(objectID string) {
	r.Lock()
	defer r.Unlock()
	delete(r.lastStatus, objectID)
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

func (r *updateReceiver) UpdateNodeStatus(status syncstatus.ConnectionStatus) {
	r.Lock()
	defer r.Unlock()
	r.nodeConnected = status == syncstatus.Online
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
