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
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
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
	objectStore     objectstore.ObjectStore
	ctx             context.Context
	ctxCancel       context.CancelFunc
	batcher         *mb.MB[*syncStatusDetails]
	spaceService    space.Service
	spaceSyncStatus SpaceStatusUpdater

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
	ids, _, err := u.objectStore.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySyncStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(domain.ObjectSyncing)),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
		},
	})
	if err != nil {
		log.Errorf("failed to update object details %s", err)
	}
	return ids
}

func (u *syncStatusUpdater) updateObjectDetails(syncStatusDetails *syncStatusDetails, objectId string) error {
	record, err := u.objectStore.GetDetails(objectId)
	if err != nil {
		return err
	}
	return u.setObjectDetails(syncStatusDetails, record.Details, objectId)
}

func (u *syncStatusUpdater) setObjectDetails(syncStatusDetails *syncStatusDetails, record *types.Struct, objectId string) error {
	status := syncStatusDetails.status
	syncError := domain.Null
	isFileStatus := false
	if fileStatus, ok := record.GetFields()[bundle.RelationKeyFileBackupStatus.String()]; ok {
		isFileStatus = true
		status, syncError = mapFileStatus(filesyncstatus.Status(int(fileStatus.GetNumberValue())))
	}
	// we want to update sync date for other stuff
	changed := u.hasRelationsChange(record, status, syncError)
	if !changed && isFileStatus {
		return nil
	}
	if !u.isLayoutSuitableForSyncRelations(record) {
		return nil
	}
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

func (u *syncStatusUpdater) hasRelationsChange(record *types.Struct, status domain.ObjectSyncStatus, syncError domain.SyncError) bool {
	var changed bool
	if record == nil || len(record.GetFields()) == 0 {
		changed = true
	}
	if pbtypes.Get(record, bundle.RelationKeySyncStatus.String()) == nil ||
		pbtypes.Get(record, bundle.RelationKeySyncError.String()) == nil {
		changed = true
	}
	if pbtypes.GetInt64(record, bundle.RelationKeySyncStatus.String()) != int64(status) {
		changed = true
	}
	if pbtypes.GetInt64(record, bundle.RelationKeySyncError.String()) != int64(syncError) {
		changed = true
	}
	return changed
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
