package space

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/periodicsync"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const (
	loopPeriodSecs = 60
	loopTimeout    = time.Second * 120
)

type localDeleter interface {
	updateRemoteStatus(ctx context.Context, spaceId string, status spaceinfo.RemoteStatus) error
	allIDs() (ids []string)
}

type deletionController struct {
	deleter localDeleter
	client  coordinatorclient.CoordinatorClient

	periodicCall periodicsync.PeriodicSync
}

func newDeletionController(
	localDeleter localDeleter,
	client coordinatorclient.CoordinatorClient) *deletionController {
	d := &deletionController{
		deleter: localDeleter,
		client:  client,
	}
	d.periodicCall = periodicsync.NewPeriodicSync(loopPeriodSecs, loopTimeout, d.loopIterate, log)
	return d
}

func (d *deletionController) Run() {
	d.periodicCall.Run()
}

func (d *deletionController) Close() {
	d.periodicCall.Close()
}

func (d *deletionController) loopIterate(ctx context.Context) error {
	d.updateStatuses(ctx)
	return nil
}

func (d *deletionController) updateStatuses(ctx context.Context) {
	ids := d.deleter.allIDs()
	remoteStatuses, err := d.client.StatusCheckMany(ctx, ids)
	if err != nil {
		log.Warn("remote status check error", zap.Error(err))
		return
	}
	convStatus := func(status coordinatorproto.SpaceStatus) spaceinfo.RemoteStatus {
		switch status {
		case coordinatorproto.SpaceStatus_SpaceStatusCreated:
			return spaceinfo.RemoteStatusOk
		case coordinatorproto.SpaceStatus_SpaceStatusPendingDeletion:
			return spaceinfo.RemoteStatusWaitingDeletion
		default:
			return spaceinfo.RemoteStatusDeleted
		}
	}
	for idx, nodeStatus := range remoteStatuses {
		if nodeStatus.Status == coordinatorproto.SpaceStatus_SpaceStatusNotExists {
			continue
		}
		remoteStatus := convStatus(nodeStatus.Status)
		err := d.deleter.updateRemoteStatus(ctx, ids[idx], remoteStatus)
		if err != nil {
			log.Warn("remote status update error", zap.Error(err), zap.String("spaceId", ids[idx]))
			return
		}
	}
}
