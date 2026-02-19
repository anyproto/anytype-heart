package deletioncontroller

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "space.deletioncontroller"

var log = logger.NewNamed(CName)

const (
	loopInterval = time.Second * 180
	loopTimeout  = time.Second * 120
)

type DeletionController interface {
	app.ComponentRunnable
	AddSpaceToDelete(spaceId string)
	UpdateCoordinatorStatus()
}

func New() DeletionController {
	return &deletionController{}
}

type SpaceManager interface {
	app.Component
	UpdateRemoteStatus(ctx context.Context, spaceStatusInfo spaceinfo.SpaceRemoteStatusInfo) error
	UpdateSharedLimits(ctx context.Context, limits int) error
	AllSpaceIds() (ids []string)
}

type deletionController struct {
	spaceManager SpaceManager
	client       coordinatorclient.CoordinatorClient
	spaceCore    spacecore.SpaceCoreService

	updater  *updateLoop
	mx       sync.Mutex
	toDelete map[string]struct{}
}

func (d *deletionController) Init(a *app.App) (err error) {
	d.client = app.MustComponent[coordinatorclient.CoordinatorClient](a)
	d.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	d.spaceManager = app.MustComponent[SpaceManager](a)
	d.updater = newUpdateLoop(d.loopIterate, loopInterval, loopTimeout)
	d.toDelete = make(map[string]struct{})
	return
}

func (d *deletionController) Name() (name string) {
	return CName
}

func (d *deletionController) AddSpaceToDelete(spaceId string) {
	d.mx.Lock()
	defer d.mx.Unlock()
	d.toDelete[spaceId] = struct{}{}
	d.updater.notify()
}

func (d *deletionController) UpdateCoordinatorStatus() {
	d.updater.notify()
}

func (d *deletionController) Run(ctx context.Context) error {
	d.updater.Run()
	return nil
}

func (d *deletionController) Close(ctx context.Context) error {
	d.updater.Close()
	return nil
}

func (d *deletionController) loopIterate(ctx context.Context) error {
	ownedIds := d.updateStatuses(ctx)
	d.mx.Lock()
	var toDeleteOwnedIds []string
	for _, id := range ownedIds {
		if _, exists := d.toDelete[id]; exists {
			toDeleteOwnedIds = append(toDeleteOwnedIds, id)
		}
	}
	d.mx.Unlock()
	d.deleteOwnedSpaces(ctx, toDeleteOwnedIds)
	return nil
}

func (d *deletionController) updateStatuses(ctx context.Context) (ownedIds []string) {
	ids := d.spaceManager.AllSpaceIds()
	remoteStatuses, limits, err := d.client.StatusCheckMany(ctx, ids)
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
	if limits != nil {
		err := d.spaceManager.UpdateSharedLimits(ctx, int(limits.SharedSpacesLimit))
		if err != nil {
			log.Warn("shared limits update error", zap.Error(err))
		}
	}
	for idx, nodeStatus := range remoteStatuses {
		if nodeStatus.Status == coordinatorproto.SpaceStatus_SpaceStatusNotExists {
			continue
		}
		isOwned := nodeStatus.Permissions == coordinatorproto.SpacePermissions_SpacePermissionsOwner
		if nodeStatus.Status == coordinatorproto.SpaceStatus_SpaceStatusCreated && isOwned {
			ownedIds = append(ownedIds, ids[idx])
		}
		remoteStatus := convStatus(nodeStatus.Status)
		shareableStatus := spaceinfo.ShareableStatusNotShareable
		if nodeStatus.IsShared {
			shareableStatus = spaceinfo.ShareableStatusShareable
		}
		info := spaceinfo.NewSpaceLocalInfo(ids[idx])
		info.SetRemoteStatus(remoteStatus).
			SetShareableStatus(shareableStatus)
		if nodeStatus.Limits != nil {
			info.SetWriteLimit(nodeStatus.Limits.WriteMembers).
				SetReadLimit(nodeStatus.Limits.ReadMembers)
		}
		err := d.spaceManager.UpdateRemoteStatus(ctx, spaceinfo.SpaceRemoteStatusInfo{
			IsOwned:   isOwned,
			LocalInfo: info,
		})
		if err != nil {
			log.Warn("remote status update error", zap.Error(err), zap.String("spaceId", ids[idx]))
			return
		}
	}
	return
}

func (d *deletionController) deleteOwnedSpaces(ctx context.Context, spaceIds []string) {
	for _, spaceId := range spaceIds {
		if err := d.spaceCore.Delete(ctx, spaceId); err != nil {
			log.Warn("space deletion error", zap.Error(err), zap.String("spaceId", spaceId))
			continue
		}
		d.mx.Lock()
		delete(d.toDelete, spaceId)
		d.mx.Unlock()
	}
}
