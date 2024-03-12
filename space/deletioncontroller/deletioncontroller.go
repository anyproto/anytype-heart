package deletioncontroller

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/acl/aclclient"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/anyproto/any-sync/coordinator/coordinatorproto"
	"github.com/anyproto/any-sync/util/periodicsync"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.space.deletioncontroller"

var log = logger.NewNamed(CName)

const (
	loopPeriodSecs = 60
	loopTimeout    = time.Second * 120
)

type DeletionController interface {
	app.ComponentRunnable
	AddSpace(spaceId string)
}

func New() DeletionController {
	return &deletionController{}
}

type spaceManager interface {
	UpdateRemoteStatus(ctx context.Context, spaceId string, status spaceinfo.RemoteStatus, isOwned bool) error
	AllSpaceIds() (ids []string)
}

type deletionController struct {
	spaceManager  spaceManager
	client        coordinatorclient.CoordinatorClient
	spaceCore     spacecore.SpaceCoreService
	joiningClient aclclient.AclJoiningClient
	keys          *accountdata.AccountKeys

	periodicCall periodicsync.PeriodicSync
	mx           sync.Mutex
	toDelete     map[string]struct{}
}

func (d *deletionController) Init(a *app.App) (err error) {
	d.client = a.MustComponent(coordinatorclient.CName).(coordinatorclient.CoordinatorClient)
	d.spaceCore = a.MustComponent(spacecore.CName).(spacecore.SpaceCoreService)
	d.joiningClient = a.MustComponent(aclclient.CName).(aclclient.AclJoiningClient)
	d.spaceManager = app.MustComponent[spaceManager](a)
	d.keys = a.MustComponent(accountservice.CName).(accountservice.Service).Account()
	d.periodicCall = periodicsync.NewPeriodicSync(loopPeriodSecs, loopTimeout, d.loopIterate, log)
	d.toDelete = make(map[string]struct{})
	return
}

func (d *deletionController) Name() (name string) {
	return CName
}

func (d *deletionController) AddSpace(spaceId string) {
	d.mx.Lock()
	defer d.mx.Unlock()
	d.toDelete[spaceId] = struct{}{}
}

func (d *deletionController) Run(ctx context.Context) error {
	d.periodicCall.Run()
	return nil
}

func (d *deletionController) Close(ctx context.Context) error {
	d.periodicCall.Close()
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
		isOwned := false
		if nodeStatus.Status == coordinatorproto.SpaceStatus_SpaceStatusCreated && nodeStatus.Permissions == coordinatorproto.SpacePermissions_SpacePermissionsOwner {
			isOwned = true
			ownedIds = append(ownedIds, ids[idx])
		}
		remoteStatus := convStatus(nodeStatus.Status)
		err := d.spaceManager.UpdateRemoteStatus(ctx, ids[idx], remoteStatus, isOwned)
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
