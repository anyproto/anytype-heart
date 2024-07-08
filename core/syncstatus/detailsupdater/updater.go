package detailsupdater

import (
	"context"
	"errors"
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
)

var log = logging.Logger(CName)

const CName = "core.syncstatus.objectsyncstatus.updater"

type syncStatusDetails struct {
	objectIds []string
	status    domain.ObjectSyncStatus
	syncError domain.SyncError
	spaceId   string
}

type Updater interface {
	app.ComponentRunnable
	UpdateDetails(objectId []string, status domain.ObjectSyncStatus, syncError domain.SyncError, spaceId string)
}

type SpaceStatusUpdater interface {
	app.Component
	SendUpdate(status *domain.SpaceSync)
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

func (u *syncStatusUpdater) UpdateDetails(objectId []string, status domain.ObjectSyncStatus, syncError domain.SyncError, spaceId string) {
	if spaceId == u.spaceService.TechSpaceId() {
		return
	}
	for _, id := range objectId {
		u.mx.Lock()
		u.entries[id] = &syncStatusDetails{
			status:    status,
			syncError: syncError,
			spaceId:   spaceId,
		}
		u.mx.Unlock()
	}
	err := u.batcher.TryAdd(&syncStatusDetails{
		objectIds: objectId,
		status:    status,
		syncError: syncError,
		spaceId:   spaceId,
	})
	if err != nil {
		log.Errorf("failed to add sync details update to queue: %s", err)
	}
}

func (u *syncStatusUpdater) updateDetails(syncStatusDetails *syncStatusDetails) {
	details := u.extractObjectDetails(syncStatusDetails)
	for _, detail := range details {
		id := pbtypes.GetString(detail.Details, bundle.RelationKeyId.String())
		err := u.setObjectDetails(syncStatusDetails, detail.Details, id)
		if err != nil {
			log.Errorf("failed to update object details %s", err)
		}
	}
}

func (u *syncStatusUpdater) extractObjectDetails(syncStatusDetails *syncStatusDetails) []database.Record {
	details, err := u.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySyncStatus.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Int64(int64(syncStatusDetails.status)),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(syncStatusDetails.spaceId),
			},
		},
	})
	if err != nil {
		log.Errorf("failed to update object details %s", err)
	}
	return details
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
	syncError := syncStatusDetails.syncError
	if fileStatus, ok := record.GetFields()[bundle.RelationKeyFileBackupStatus.String()]; ok {
		status, syncError = mapFileStatus(filesyncstatus.Status(int(fileStatus.GetNumberValue())))
	}
	changed := u.hasRelationsChange(record, status, syncError)
	if !changed {
		return nil
	}
	if !u.isLayoutSuitableForSyncRelations(record) {
		return nil
	}
	spc, err := u.spaceService.Get(u.ctx, syncStatusDetails.spaceId)
	if err != nil {
		return err
	}
	spaceStatus := mapObjectSyncToSpaceSyncStatus(status, syncError)
	defer u.sendSpaceStatusUpdate(err, syncStatusDetails, spaceStatus, syncError)
	err = spc.DoLockedIfNotExists(objectId, func() error {
		return u.objectStore.ModifyObjectDetails(objectId, func(details *types.Struct) (*types.Struct, bool, error) {
			if details == nil || details.Fields == nil {
				details = &types.Struct{Fields: map[string]*types.Value{}}
			}
			details.Fields[bundle.RelationKeySyncStatus.String()] = pbtypes.Int64(int64(status))
			details.Fields[bundle.RelationKeySyncError.String()] = pbtypes.Int64(int64(syncError))
			details.Fields[bundle.RelationKeySyncDate.String()] = pbtypes.Int64(time.Now().Unix())
			return details, true, nil
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

func mapObjectSyncToSpaceSyncStatus(status domain.ObjectSyncStatus, syncError domain.SyncError) domain.SpaceSyncStatus {
	switch status {
	case domain.ObjectSynced:
		return domain.Synced
	case domain.ObjectSyncing, domain.ObjectQueued:
		return domain.Syncing
	case domain.ObjectError:
		// don't send error to space if file were oversized
		if syncError != domain.Oversized {
			return domain.Error
		}
	}
	return domain.Synced
}

func (u *syncStatusUpdater) sendSpaceStatusUpdate(err error, syncStatusDetails *syncStatusDetails, status domain.SpaceSyncStatus, syncError domain.SyncError) {
	if err == nil {
		u.spaceSyncStatus.SendUpdate(domain.MakeSyncStatus(syncStatusDetails.spaceId, status, syncError, domain.Objects))
	}
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
	for {
		status, err := u.batcher.WaitOne(u.ctx)
		if err != nil {
			return
		}
		for _, id := range status.objectIds {
			u.mx.Lock()
			objectStatus := u.entries[id]
			delete(u.entries, id)
			u.mx.Unlock()
			if objectStatus != nil {
				err := u.updateObjectDetails(objectStatus, id)
				if err != nil {
					log.Errorf("failed to update details %s", err)
				}
			}
		}
		if len(status.objectIds) == 0 {
			u.updateDetails(status)
		}
	}
}
