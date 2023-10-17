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
	loopTimeout    = time.Second * 10
)

type loopAction int

const (
	loopActionNothing = iota
	loopActionDelete
	loopActionDeleteRemotely
)

type localDeleter interface {
	startDelete(ctx context.Context, id string) error
	setStatus(ctx context.Context, info spaceinfo.SpaceInfo) (err error)
	allStatuses() (statuses []spaceinfo.SpaceInfo)
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
	statuses := d.updateStatuses(ctx)
	d.checkStatuses(ctx, statuses)
	return nil
}

func (d *deletionController) updateStatuses(ctx context.Context) (statuses []spaceinfo.SpaceInfo) {
	localStatuses := d.deleter.allStatuses()
	spaceIDs := make([]string, 0, len(localStatuses))
	for _, status := range localStatuses {
		spaceIDs = append(spaceIDs, status.SpaceID)
	}
	remoteStatuses, err := d.client.StatusCheckMany(ctx, spaceIDs)
	if err != nil {
		log.Warn("remote status check error", zap.Error(err))
		return localStatuses
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
	for idx, remoteStatus := range remoteStatuses {
		err := d.deleter.setStatus(ctx, localStatuses[idx])
		if err != nil {
			log.Warn("local status update error", zap.Error(err), zap.String("spaceId", localStatuses[idx].SpaceID))
			return localStatuses
		}
		localStatuses[idx].RemoteStatus = convStatus(remoteStatus.Status)
	}
	return localStatuses
}

func (d *deletionController) checkStatuses(ctx context.Context, localStatuses []spaceinfo.SpaceInfo) {
	for _, status := range localStatuses {
		if d.shouldDelete(status) {
			err := d.deleter.startDelete(ctx, status.SpaceID)
			if err != nil {
				log.Warn("local delete error", zap.Error(err), zap.String("spaceId", status.SpaceID))
			}
		}
	}
}

func (d *deletionController) shouldDelete(localStatus spaceinfo.SpaceInfo) bool {
	if localStatus.AccountStatus == spaceinfo.AccountStatusDeleted {
		return localStatus.RemoteStatus != spaceinfo.RemoteStatusDeleted || localStatus.LocalStatus != spaceinfo.LocalStatusMissing
	}
	if localStatus.RemoteStatus == spaceinfo.RemoteStatusDeleted {
		return localStatus.LocalStatus != spaceinfo.LocalStatusMissing
	}
	return false
}
