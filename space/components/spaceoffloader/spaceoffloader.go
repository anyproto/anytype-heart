package spaceoffloader

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/components/dependencies"
	"github.com/anyproto/anytype-heart/space/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/process/modechanger"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.common.spaceoffloader"

var log = logger.NewNamed(CName)

const deleteStorageLockTimeout = time.Second * 10

type SpaceOffloader interface {
	app.ComponentRunnable
	WaitOffload(ctx context.Context) (err error)
}

func New() SpaceOffloader {
	return &spaceOffloader{}
}

type spaceOffloader struct {
	status         *spacestatus.SpaceStatus
	offloading     *offloadingSpace
	fileOffloader  dependencies.FileOffloader
	storageService storage.ClientStorage
	indexer        dependencies.SpaceIndexer
	spaceCore      spacecore.SpaceCoreService
	ctx            context.Context
	cancel         context.CancelFunc
	offloaded      atomic.Bool
}

func (o *spaceOffloader) Init(a *app.App) (err error) {
	o.status = app.MustComponent[*spacestatus.SpaceStatus](a)
	o.fileOffloader = app.MustComponent[dependencies.FileOffloader](a)
	o.storageService = app.MustComponent[storage.ClientStorage](a)
	o.indexer = app.MustComponent[dependencies.SpaceIndexer](a)
	o.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	o.ctx, o.cancel = context.WithCancel(context.Background())
	return nil
}

func (o *spaceOffloader) Name() (name string) {
	return CName
}

func (o *spaceOffloader) Run(ctx context.Context) (err error) {
	o.status.Lock()
	persistentStatus := o.status.GetPersistentStatus()
	if persistentStatus != spaceinfo.AccountStatusDeleted {
		persistentStatus = spaceinfo.AccountStatusDeleted
		err := o.status.SetPersistentStatus(ctx, persistentStatus)
		if err != nil {
			o.status.Unlock()
			return err
		}
	}
	localStatus := o.status.GetLocalStatus()
	remoteStatus := o.status.GetRemoteStatus()
	o.status.Unlock()
	if !remoteStatus.IsDeleted() {
		err := o.spaceCore.Delete(ctx, o.status.SpaceId)
		if err != nil {
			log.Debug("network delete error", zap.Error(err), zap.String("spaceId", o.status.SpaceId))
		}
	}
	if localStatus == spaceinfo.LocalStatusMissing {
		return nil
	}
	o.offloading = newOffloadingSpace(o.ctx, o.status.SpaceId, o)
	return nil
}

func (o *spaceOffloader) Close(ctx context.Context) (err error) {
	o.cancel()
	if o.offloading != nil {
		<-o.offloading.loadCh
	}
	return nil
}

func (o *spaceOffloader) CanTransition(next modechanger.Mode) bool {
	return false
}

func (o *spaceOffloader) startOffload(ctx context.Context, id string) (err error) {
	o.offloading = newOffloadingSpace(ctx, id, o)
	return nil
}

func (o *spaceOffloader) onOffload(id string, offloadErr error) {
	if offloadErr != nil {
		log.Warn("offload error", zap.Error(offloadErr), zap.String("spaceId", id))
		return
	}
	o.status.Lock()
	defer o.status.Unlock()
	if err := o.status.UpdateLocalStatus(o.ctx, spaceinfo.LocalStatusMissing); err != nil {
		log.Debug("set status error", zap.Error(err), zap.String("spaceId", id))
	}
	o.offloaded.Store(true)
}

func (o *spaceOffloader) WaitOffload(ctx context.Context) (err error) {
	if o.offloaded.Load() {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-o.offloading.loadCh:
		return o.offloading.loadErr
	}
}

func (o *spaceOffloader) offload(ctx context.Context, id string) (err error) {
	ctx, cancel := context.WithTimeout(ctx, deleteStorageLockTimeout)
	err = o.storageService.DeleteSpaceStorage(ctx, id)
	cancel()
	if err != nil {
		return
	}
	err = o.fileOffloader.FilesSpaceOffload(ctx, id)
	if err != nil {
		return err
	}
	return o.indexer.RemoveIndexes(id)
}
