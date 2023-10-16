package space

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/periodicsync"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const (
	loopPeriodSecs = 60
	loopTimeout    = time.Second * 10
)

type loopAction int

const (
	loopActionNothing = iota
	loopActionDeleteLocally
	loopActionDeleteRemotely
)

type localDeleter interface {
	Delete(ctx context.Context, id string) (err error)
	allStatuses() (statuses []spaceinfo.SpaceInfo)
}

type remoteDeleter interface {
	Delete(ctx context.Context, spaceID string) (payload spacecore.NetworkStatus, err error)
}

type deletionController struct {
	localDeleter  localDeleter
	remoteDeleter remoteDeleter
	client        coordinatorclient.CoordinatorClient

	periodicCall periodicsync.PeriodicSync
}

func newDeletionController(
	localDeleter localDeleter,
	remoteDeleter remoteDeleter,
	client coordinatorclient.CoordinatorClient) *deletionController {
	d := &deletionController{
		localDeleter:  localDeleter,
		remoteDeleter: remoteDeleter,
		client:        client,
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

func (d *deletionController) loopIterate(ctx context.Context) (err error) {
	localStatuses := d.localDeleter.allStatuses()
	spaceIDs := make([]string, 0, len(localStatuses))
	for _, status := range localStatuses {
		spaceIDs = append(spaceIDs, status.SpaceID)
	}
	remoteStatuses, err := d.client.StatusCheckMany(ctx, spaceIDs)
	if err != nil {
		return
	}
	for idx, remoteStatus := range remoteStatuses {
		localStatus := localStatuses[idx]
		action := d.compareStatuses(localStatuses[idx], remoteStatus.Status)
		switch action {
		case loopActionDeleteLocally:
			err = d.localDeleter.Delete(ctx, localStatus.SpaceID)
			if err != nil {
				log.Warn("local delete error", zap.Error(err), zap.String("spaceId", localStatus.SpaceID))
			}
		case loopActionDeleteRemotely:
			_, err = d.remoteDeleter.Delete(ctx, localStatus.SpaceID)
			if err != nil {
				log.Warn("remote delete error", zap.Error(err), zap.String("spaceId", localStatus.SpaceID))
			}
		}
	}
	return
}

func (d *deletionController) compareStatuses(localStatus spaceinfo.SpaceInfo, remoteStatus coordinatorproto.SpaceStatus) loopAction {
	if localStatus.LocalStatus == spaceinfo.LocalStatusOk && remoteStatus == coordinatorproto.SpaceStatus_SpaceStatusDeleted {
		return loopActionDeleteLocally
	}
	if localStatus.AccountStatus == spaceinfo.AccountStatusDeleted && remoteStatus != coordinatorproto.SpaceStatus_SpaceStatusDeleted {
		return loopActionDeleteRemotely
	}
	return loopActionNothing
}
