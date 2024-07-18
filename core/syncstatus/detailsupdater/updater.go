package detailsupdater

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/detailsupdater/helper"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscritions"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var log = logging.Logger(CName)

const CName = "core.syncstatus.objectsyncstatus.updater"

type syncStatusDetails struct {
	objectId            string
	markAllSyncedExcept []string
	status              domain.ObjectSyncStatus
	spaceId             string
}

type Updater interface {
	app.ComponentRunnable
	UpdateSpaceDetails(existing, missing []string, spaceId string)
	UpdateDetails(objectId string, status domain.ObjectSyncStatus, spaceId string)
}

type SpaceStatusUpdater interface {
	app.Component
	Refresh(spaceId string)
	UpdateMissingIds(spaceId string, ids []string)
}

type syncStatusUpdater struct {
	objectStore       objectstore.ObjectStore
	ctx               context.Context
	ctxCancel         context.CancelFunc
	batcher           *mb.MB[*syncStatusDetails]
	spaceService      space.Service
	spaceSyncStatus   SpaceStatusUpdater
	syncSubscriptions syncsubscritions.SyncSubscriptions

	entries map[string]*syncStatusDetails
	mx      sync.Mutex

	finish chan struct{}
}

func NewUpdater() Updater {
	return &syncStatusUpdater{
		batcher: mb.New[*syncStatusDetails](0),
		finish:  make(chan struct{}),
		entries: make(map[string]*syncStatusDetails, 0),
	}
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
	u.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	u.spaceService = app.MustComponent[space.Service](a)
	u.spaceSyncStatus = app.MustComponent[SpaceStatusUpdater](a)
	u.syncSubscriptions = app.MustComponent[syncsubscritions.SyncSubscriptions](a)
	return nil
}

func (u *syncStatusUpdater) Name() (name string) {
	return CName
}

func (u *syncStatusUpdater) UpdateDetails(objectId string, status domain.ObjectSyncStatus, spaceId string) {
	if spaceId == u.spaceService.TechSpaceId() {
		return
	}
	u.mx.Lock()
	u.entries[objectId] = &syncStatusDetails{
		objectId: objectId,
		status:   status,
		spaceId:  spaceId,
	}
	u.mx.Unlock()
	err := u.batcher.TryAdd(&syncStatusDetails{
		objectId: objectId,
		status:   status,
		spaceId:  spaceId,
	})
	if err != nil {
		log.Errorf("failed to add sync details update to queue: %s", err)
	}
}

func (u *syncStatusUpdater) UpdateSpaceDetails(existing, missing []string, spaceId string) {
	if spaceId == u.spaceService.TechSpaceId() {
		return
	}
	u.spaceSyncStatus.UpdateMissingIds(spaceId, missing)
	err := u.batcher.TryAdd(&syncStatusDetails{
		markAllSyncedExcept: existing,
		status:              domain.ObjectSyncing,
		spaceId:             spaceId,
	})
	fmt.Println("[x]: sending update to batcher, len(existing)", len(existing), "len(missing)", len(missing), "spaceId", spaceId)
	if err != nil {
		log.Errorf("failed to add sync details update to queue: %s", err)
	}
}

func (u *syncStatusUpdater) getSyncingObjects(spaceId string) []string {
	sub, err := u.syncSubscriptions.GetSubscription(spaceId)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, sub.GetObjectSubscription().Len())
	sub.GetObjectSubscription().Iterate(func(id string, _ struct{}) bool {
		ids = append(ids, id)
		return true
	})
	return ids
}

func (u *syncStatusUpdater) updateObjectDetails(syncStatusDetails *syncStatusDetails, objectId string) error {
	return u.setObjectDetails(syncStatusDetails, objectId)
}

func (u *syncStatusUpdater) setObjectDetails(syncStatusDetails *syncStatusDetails, objectId string) error {
	status := syncStatusDetails.status
	syncError := domain.Null
	spc, err := u.spaceService.Get(u.ctx, syncStatusDetails.spaceId)
	if err != nil {
		return err
	}
	defer u.spaceSyncStatus.Refresh(syncStatusDetails.spaceId)
	err = spc.DoLockedIfNotExists(objectId, func() error {
		return u.objectStore.ModifyObjectDetails(objectId, func(details *types.Struct) (*types.Struct, error) {
			if details == nil || details.Fields == nil {
				details = &types.Struct{Fields: map[string]*types.Value{}}
			}
			if !u.isLayoutSuitableForSyncRelations(details) {
				return details, nil
			}
			if fileStatus, ok := details.GetFields()[bundle.RelationKeyFileBackupStatus.String()]; ok {
				status, syncError = mapFileStatus(filesyncstatus.Status(int(fileStatus.GetNumberValue())))
			}
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
	return spc.DoCtx(u.ctx, objectId, func(sb smartblock.SmartBlock) error {
		return u.setSyncDetails(sb, status, syncError)
	})
}

func (u *syncStatusUpdater) isLayoutSuitableForSyncRelations(details *types.Struct) bool {
	layoutsWithoutSyncRelations := []float64{
		float64(model.ObjectType_participant),
		float64(model.ObjectType_dashboard),
		float64(model.ObjectType_spaceView),
		float64(model.ObjectType_space),
		float64(model.ObjectType_date),
	}
	layout := details.Fields[bundle.RelationKeyLayout.String()].GetNumberValue()
	return !slices.Contains(layoutsWithoutSyncRelations, layout)
}

func mapFileStatus(status filesyncstatus.Status) (domain.ObjectSyncStatus, domain.SyncError) {
	var syncError domain.SyncError
	switch status {
	case filesyncstatus.Syncing:
		return domain.ObjectSyncing, domain.Null
	case filesyncstatus.Queued:
		return domain.ObjectQueued, domain.Null
	case filesyncstatus.Limited:
		syncError = domain.Oversized
		return domain.ObjectError, syncError
	case filesyncstatus.Unknown:
		syncError = domain.NetworkError
		return domain.ObjectError, syncError
	default:
		return domain.ObjectSynced, domain.Null
	}
}

func (u *syncStatusUpdater) setSyncDetails(sb smartblock.SmartBlock, status domain.ObjectSyncStatus, syncError domain.SyncError) error {
	if !slices.Contains(helper.SyncRelationsSmartblockTypes(), sb.Type()) {
		return nil
	}
	details := sb.CombinedDetails()
	if !u.isLayoutSuitableForSyncRelations(details) {
		return nil
	}
	if fileStatus, ok := details.GetFields()[bundle.RelationKeyFileBackupStatus.String()]; ok {
		status, syncError = mapFileStatus(filesyncstatus.Status(int(fileStatus.GetNumberValue())))
	}
	if d, ok := sb.(basic.DetailsSettable); ok {
		syncStatusDetails := []*model.Detail{
			{
				Key:   bundle.RelationKeySyncStatus.String(),
				Value: pbtypes.Int64(int64(status)),
			},
			{
				Key:   bundle.RelationKeySyncError.String(),
				Value: pbtypes.Int64(int64(syncError)),
			},
			{
				Key:   bundle.RelationKeySyncDate.String(),
				Value: pbtypes.Int64(time.Now().Unix()),
			},
		}
		return d.SetDetails(nil, syncStatusDetails, false)
	}
	return nil
}

func (u *syncStatusUpdater) processEvents() {
	defer close(u.finish)
	updateSpecificObject := func(details *syncStatusDetails) {
		u.mx.Lock()
		objectStatus := u.entries[details.objectId]
		delete(u.entries, details.objectId)
		u.mx.Unlock()
		if objectStatus != nil {
			err := u.updateObjectDetails(objectStatus, details.objectId)
			if err != nil {
				log.Errorf("failed to update details %s", err)
			}
		}
	}
	syncAllObjectsExcept := func(details *syncStatusDetails) {
		ids := u.getSyncingObjects(details.spaceId)
		removed, added := slice.DifferenceRemovedAdded(details.markAllSyncedExcept, ids)
		if len(removed)+len(added) == 0 {
			u.spaceSyncStatus.Refresh(details.spaceId)
			return
		}
		fmt.Println("[x]: marking synced, len(synced)", len(added), "len(syncing)", len(removed), "spaceId", details.spaceId)
		details.status = domain.ObjectSynced
		for _, id := range added {
			err := u.updateObjectDetails(details, id)
			if err != nil {
				log.Errorf("failed to update details %s", err)
			}
		}
		details.status = domain.ObjectSyncing
		for _, id := range removed {
			err := u.updateObjectDetails(details, id)
			if err != nil {
				log.Errorf("failed to update details %s", err)
			}
		}
	}
	for {
		status, err := u.batcher.WaitOne(u.ctx)
		if err != nil {
			return
		}
		if status.objectId == "" {
			syncAllObjectsExcept(status)
		} else {
			updateSpecificObject(status)
		}
	}
}
