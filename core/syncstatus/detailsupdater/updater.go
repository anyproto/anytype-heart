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

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/detailsupdater/helper"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscriptions"
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
	objectId string
	status   domain.ObjectSyncStatus
	spaceId  string
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
	batcher           *mb.MB[string]
	spaceService      space.Service
	spaceSyncStatus   SpaceStatusUpdater
	syncSubscriptions syncsubscriptions.SyncSubscriptions

	entries map[string]*syncStatusDetails
	mx      sync.Mutex

	finish chan struct{}
}

func New() Updater {
	return &syncStatusUpdater{
		batcher: mb.New[string](0),
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
	u.syncSubscriptions = app.MustComponent[syncsubscriptions.SyncSubscriptions](a)
	return nil
}

func (u *syncStatusUpdater) Name() (name string) {
	return CName
}

func (u *syncStatusUpdater) UpdateDetails(objectId string, status domain.ObjectSyncStatus, spaceId string) {
	if spaceId == u.spaceService.TechSpaceId() {
		return
	}
	err := u.addToQueue(&syncStatusDetails{
		objectId: objectId,
		status:   status,
		spaceId:  spaceId,
	})
	if err != nil {
		log.Errorf("failed to add sync details update to queue: %s", err)
	}
}

func (u *syncStatusUpdater) addToQueue(details *syncStatusDetails) error {
	u.mx.Lock()
	u.entries[details.objectId] = details
	u.mx.Unlock()
	return u.batcher.TryAdd(details.objectId)
}

func (u *syncStatusUpdater) processEvents() {
	defer close(u.finish)

	for {
		objectId, err := u.batcher.WaitOne(u.ctx)
		if err != nil {
			return
		}
		u.updateSpecificObject(objectId)
	}
}

func (u *syncStatusUpdater) updateSpecificObject(objectId string) {
	u.mx.Lock()
	objectStatus := u.entries[objectId]
	delete(u.entries, objectId)
	u.mx.Unlock()

	if objectStatus != nil {
		err := u.updateObjectDetails(objectStatus, objectId)
		if err != nil {
			log.Errorf("failed to update details %s", err)
		}
	}
}

func (u *syncStatusUpdater) UpdateSpaceDetails(existing, missing []string, spaceId string) {
	if spaceId == u.spaceService.TechSpaceId() {
		return
	}
	u.spaceSyncStatus.UpdateMissingIds(spaceId, missing)
	ids := u.getSyncingObjects(spaceId)

	// removed contains ids that are not yet marked as syncing
	// added contains ids that were syncing, but appeared as synced, because they are not in existing list
	removed, added := slice.DifferenceRemovedAdded(existing, ids)
	if len(removed)+len(added) == 0 {
		u.spaceSyncStatus.Refresh(spaceId)
		return
	}
	for _, id := range added {
		err := u.addToQueue(&syncStatusDetails{
			objectId: id,
			status:   domain.ObjectSyncStatusSynced,
			spaceId:  spaceId,
		})
		if err != nil {
			log.Errorf("failed to add sync details update to queue: %s", err)
		}
	}
	for _, id := range removed {
		err := u.addToQueue(&syncStatusDetails{
			objectId: id,
			status:   domain.ObjectSyncStatusSyncing,
			spaceId:  spaceId,
		})
		if err != nil {
			log.Errorf("failed to add sync details update to queue: %s", err)
		}
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
	status := syncStatusDetails.status
	syncError := domain.SyncErrorNull
	spc, err := u.spaceService.Get(u.ctx, syncStatusDetails.spaceId)
	if err != nil {
		return err
	}
	defer u.spaceSyncStatus.Refresh(syncStatusDetails.spaceId)
	err = spc.DoLockedIfNotExists(objectId, func() error {
		return u.objectStore.ModifyObjectDetails(objectId, func(details *types.Struct) (*types.Struct, bool, error) {
			if details == nil || details.Fields == nil {
				details = &types.Struct{Fields: map[string]*types.Value{}}
			}
			if !u.isLayoutSuitableForSyncRelations(details) {
				return details, false, nil
			}
			if fileStatus, ok := details.GetFields()[bundle.RelationKeyFileBackupStatus.String()]; ok {
				status, syncError = getSyncStatusForFile(status, syncError, filesyncstatus.Status(int(fileStatus.GetNumberValue())))
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

func (u *syncStatusUpdater) setSyncDetails(sb smartblock.SmartBlock, status domain.ObjectSyncStatus, syncError domain.SyncError) error {
	if !slices.Contains(helper.SyncRelationsSmartblockTypes(), sb.Type()) {
		return nil
	}
	if !u.isLayoutSuitableForSyncRelations(sb.Details()) {
		return nil
	}
	st := sb.NewState()
	if fileStatus, ok := st.Details().GetFields()[bundle.RelationKeyFileBackupStatus.String()]; ok {
		status, syncError = getSyncStatusForFile(status, syncError, filesyncstatus.Status(int(fileStatus.GetNumberValue())))
	}
	st.SetDetailAndBundledRelation(bundle.RelationKeySyncStatus, pbtypes.Int64(int64(status)))
	st.SetDetailAndBundledRelation(bundle.RelationKeySyncError, pbtypes.Int64(int64(syncError)))
	st.SetDetailAndBundledRelation(bundle.RelationKeySyncDate, pbtypes.Int64(time.Now().Unix()))

	return sb.Apply(st, smartblock.KeepInternalFlags /* do not erase flags */)
}

var suitableLayouts = map[model.ObjectTypeLayout]struct{}{
	model.ObjectType_basic:          {},
	model.ObjectType_profile:        {},
	model.ObjectType_todo:           {},
	model.ObjectType_set:            {},
	model.ObjectType_objectType:     {},
	model.ObjectType_relation:       {},
	model.ObjectType_file:           {},
	model.ObjectType_image:          {},
	model.ObjectType_note:           {},
	model.ObjectType_bookmark:       {},
	model.ObjectType_relationOption: {},
	model.ObjectType_collection:     {},
	model.ObjectType_audio:          {},
	model.ObjectType_video:          {},
	model.ObjectType_pdf:            {},
}

func (u *syncStatusUpdater) isLayoutSuitableForSyncRelations(details *types.Struct) bool {
	layout := model.ObjectTypeLayout(pbtypes.GetInt64(details, bundle.RelationKeyLayout.String()))
	_, ok := suitableLayouts[layout]
	return ok
}

func getSyncStatusForFile(objectStatus domain.ObjectSyncStatus, objectSyncError domain.SyncError, fileStatus filesyncstatus.Status) (domain.ObjectSyncStatus, domain.SyncError) {
	statusFromFile, errFromFile := mapFileStatus(fileStatus)
	// If file status is synced, then prioritize object's status, otherwise pick file status
	if statusFromFile != domain.ObjectSyncStatusSynced {
		objectStatus = statusFromFile
	}
	if errFromFile != domain.SyncErrorNull {
		objectSyncError = errFromFile
	}
	return objectStatus, objectSyncError
}

func mapFileStatus(status filesyncstatus.Status) (domain.ObjectSyncStatus, domain.SyncError) {
	switch status {
	case filesyncstatus.Syncing:
		return domain.ObjectSyncStatusSyncing, domain.SyncErrorNull
	case filesyncstatus.Queued:
		return domain.ObjectSyncStatusQueued, domain.SyncErrorNull
	case filesyncstatus.Limited:
		return domain.ObjectSyncStatusError, domain.SyncErrorOversized
	case filesyncstatus.Unknown:
		return domain.ObjectSyncStatusError, domain.SyncErrorNetworkError
	default:
		return domain.ObjectSyncStatusSynced, domain.SyncErrorNull
	}
}
