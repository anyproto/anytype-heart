package detailsupdater

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger(CName)

const CName = "core.syncstatus.objectsyncstatus.updater"

type syncStatusDetails struct {
	objectId  string
	status    domain.SyncStatus
	syncError domain.SyncError
}

type Updater interface {
	app.ComponentRunnable
	UpdateDetails(objectId string, status domain.SyncStatus, syncError domain.SyncError)
}

type syncStatusUpdater struct {
	objectGetter cache.ObjectGetter
	objectStore  objectstore.ObjectStore
	ctx          context.Context
	ctxCancel    context.CancelFunc
	batcher      *mb.MB[*syncStatusDetails]
	finish       chan struct{}
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
	return nil
}

func (u *syncStatusUpdater) Name() (name string) {
	return CName
}

func (u *syncStatusUpdater) UpdateDetails(objectId string, status domain.SyncStatus, syncError domain.SyncError) {
	err := u.batcher.Add(context.Background(), &syncStatusDetails{
		objectId:  objectId,
		status:    status,
		syncError: syncError,
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
	return cache.Do(u.objectGetter, objectId, func(sb basic.DetailsSettable) error {
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
		return sb.SetDetails(nil, syncStatusDetails, false)
	})
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
