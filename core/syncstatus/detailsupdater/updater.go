package detailsupdater

import (
	"context"
	"errors"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger(CName)

const CName = "core.syncstatus.objectsyncstatus.updater"

type syncStatusDetails struct {
	objectId  string
	status    domain.SyncStatus
	syncError domain.SyncError
	spaceId   string
}

type Updater interface {
	app.ComponentRunnable
	UpdateDetails(objectId string, status domain.SyncStatus, syncError domain.SyncError, spaceId string)
}

type syncStatusUpdater struct {
	objectGetter cache.ObjectGetter
	objectStore  objectstore.ObjectStore
	ctx          context.Context
	ctxCancel    context.CancelFunc
	batcher      *mb.MB[*syncStatusDetails]
	spaceService space.Service

	finish chan struct{}
}

func NewUpdater() Updater {
	return &syncStatusUpdater{batcher: mb.New[*syncStatusDetails](0), finish: make(chan struct{})}
}

func (u *syncStatusUpdater) Run(ctx context.Context) (err error) {
	u.ctx, u.ctxCancel = context.WithCancel(context.Background())
	go u.processEvents()
	return nil
}

func (u *syncStatusUpdater) Close(ctx context.Context) (err error) {
	if u.ctxCancel != nil {
		u.ctxCancel()
	}
	<-u.finish
	return u.batcher.Close()
}

func (u *syncStatusUpdater) Init(a *app.App) (err error) {
	u.objectGetter = app.MustComponent[cache.ObjectGetter](a)
	u.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	u.spaceService = app.MustComponent[space.Service](a)

	return nil
}

func (u *syncStatusUpdater) Name() (name string) {
	return CName
}

func (u *syncStatusUpdater) UpdateDetails(objectId string, status domain.SyncStatus, syncError domain.SyncError, spaceId string) {
	err := u.batcher.Add(context.Background(), &syncStatusDetails{
		objectId:  objectId,
		status:    status,
		syncError: syncError,
		spaceId:   spaceId,
	})
	if err != nil {
		log.Errorf("failed to add sync details update to queue: %s", err)
	}
}

func (u *syncStatusUpdater) updateDetails(syncStatusDetails *syncStatusDetails) error {
	objectId := syncStatusDetails.objectId
	record, err := u.objectStore.GetDetails(objectId)
	if err != nil {
		return err
	}
	status := syncStatusDetails.status
	syncError := syncStatusDetails.syncError
	changed := u.hasRelationsChange(record, status, syncError)
	if !changed {
		return nil
	}
	spc, err := u.spaceService.Get(context.Background(), syncStatusDetails.spaceId)
	if err != nil {
		return err
	}
	err = spc.DoLockedIfNotExists(objectId, func() error {
		return u.objectStore.ModifyObjectDetails(objectId, func(details *types.Struct) (*types.Struct, error) {
			details.Fields[bundle.RelationKeySyncStatus.String()] = pbtypes.Int64(int64(status))
			details.Fields[bundle.RelationKeySyncError.String()] = pbtypes.Int64(int64(syncError))
			details.Fields[bundle.RelationKeySyncDate.String()] = pbtypes.Int64(time.Now().Unix())
			return details, nil
		})
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, ocache.ErrExists) {
		return err
	}
	return spc.Do(objectId, func(sb smartblock.SmartBlock) error {
		return u.setSyncDetails(sb, status, syncError)
	})
}

func (u *syncStatusUpdater) setSyncDetails(sb smartblock.SmartBlock, status domain.SyncStatus, syncError domain.SyncError) error {
	if d, ok := sb.(basic.DetailsSettable); ok {
		syncStatusDetails := []*model.Detail{
			{
				Key:   bundle.RelationKeySyncStatus.String(),
				Value: pbtypes.Int64(int64(status)),
			},
		}
		syncStatusDetails = append(syncStatusDetails, &model.Detail{
			Key:   bundle.RelationKeySyncError.String(),
			Value: pbtypes.Int64(int64(syncError)),
		})
		syncStatusDetails = append(syncStatusDetails, &model.Detail{
			Key:   bundle.RelationKeySyncDate.String(),
			Value: pbtypes.Int64(time.Now().Unix()),
		})
		return d.SetDetails(nil, syncStatusDetails, false)
	}
	return nil
}

func (u *syncStatusUpdater) hasRelationsChange(record *model.ObjectDetails, status domain.SyncStatus, syncError domain.SyncError) bool {
	var changed bool
	if record == nil || record.Details == nil || len(record.Details.GetFields()) == 0 {
		changed = true
	}
	if pbtypes.GetInt64(record.Details, bundle.RelationKeySyncStatus.String()) != int64(status) {
		changed = true
	}
	if pbtypes.GetInt64(record.Details, bundle.RelationKeySyncError.String()) != int64(syncError) {
		changed = true
	}
	return changed
}

func (u *syncStatusUpdater) processEvents() {
	defer close(u.finish)
	for {
		status, err := u.batcher.WaitOne(u.ctx)
		if err != nil {
			return
		}
		err = u.updateDetails(status)
		if err != nil {
			log.Errorf("failed to update sync details %s", err)
			continue
		}
	}
}
