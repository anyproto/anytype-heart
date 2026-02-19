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

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block/editor/components"
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
	"github.com/anyproto/anytype-heart/util/slice"
)

var log = logging.Logger(CName)

const CName = "core.syncstatus.objectsyncstatus.updater"

const batchTime = 500 * time.Millisecond

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
	accountService    account.Service
	myIdentity        string

	entries map[string]*syncStatusDetails
	lock    sync.Mutex

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
	u.myIdentity = u.accountService.AccountID()
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
	u.accountService = app.MustComponent[account.Service](a)
	return nil
}

func (u *syncStatusUpdater) Name() (name string) {
	return CName
}

func (u *syncStatusUpdater) UpdateDetails(objectId string, status domain.ObjectSyncStatus, spaceId string) {
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
	u.lock.Lock()
	_, ok := u.entries[details.objectId]
	u.entries[details.objectId] = details
	u.lock.Unlock()
	if !ok {
		return u.batcher.TryAdd(details.objectId)
	}
	return nil
}

func (u *syncStatusUpdater) processEvents() {
	defer close(u.finish)

	for {
		objectIds, err := u.batcher.Wait(u.ctx)
		if err != nil {
			return
		}
		now := time.Now()
		for _, objectId := range objectIds {
			u.updateSpecificObject(objectId)
		}

		sleepDuration := batchTime - time.Since(now)
		if sleepDuration <= 0 {
			continue
		}
		time.Sleep(sleepDuration)
	}
}

func (u *syncStatusUpdater) updateSpecificObject(objectId string) {
	u.lock.Lock()
	objectStatus, ok := u.entries[objectId]
	delete(u.entries, objectId)
	u.lock.Unlock()

	if ok {
		err := u.updateObjectDetails(objectStatus, objectId)
		if err != nil {
			log.Errorf("failed to update details %s", err)
		}
	}
}

func (u *syncStatusUpdater) UpdateSpaceDetails(existing, missing []string, spaceId string) {
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
		return u.objectStore.SpaceIndex(syncStatusDetails.spaceId).ModifyObjectDetails(objectId, func(details *domain.Details) (*domain.Details, bool, error) {
			if details == nil {
				details = domain.NewDetails()
			}

			// Force updating via cache
			if details.GetInt64(bundle.RelationKeyResolvedLayout) == int64(model.ObjectType_chatDerived) {
				return nil, false, ocache.ErrExists
			}

			// todo: make the checks consistent here and in setSyncDetails
			if !u.isLayoutSuitableForSyncRelations(details) {
				return details, false, nil
			}

			status, syncError = u.tryUpdateFromFileBackupStatus(status, syncError, details, details, syncStatusDetails.spaceId)

			details.SetInt64(bundle.RelationKeySyncStatus, int64(status))
			details.SetInt64(bundle.RelationKeySyncError, int64(syncError))
			details.SetInt64(bundle.RelationKeySyncDate, time.Now().Unix())
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
	if comp, ok := sb.(components.SyncStatusHandler); ok {
		comp.HandleSyncStatusUpdate(sb.Tree().Heads(), status, syncError)
	}

	if !slices.Contains(helper.SyncRelationsSmartblockTypes(), sb.Type()) {
		if sb.LocalDetails().Has(bundle.RelationKeySyncStatus) {
			// do cleanup because of previous sync relations indexation problem
			st := sb.NewState()
			st.LocalDetails().Delete(bundle.RelationKeySyncDate)
			st.LocalDetails().Delete(bundle.RelationKeySyncStatus)
			st.LocalDetails().Delete(bundle.RelationKeySyncError)
			return sb.Apply(st, smartblock.KeepInternalFlags)
		}
		return nil
	}
	st := sb.NewState()
	if !u.isLayoutSuitableForSyncRelations(sb.LocalDetails()) {
		return nil
	}

	status, syncError = u.tryUpdateFromFileBackupStatus(status, syncError, sb.LocalDetails(), sb.Details(), sb.SpaceID())

	st.SetDetailAndBundledRelation(bundle.RelationKeySyncStatus, domain.Int64(status))
	st.SetDetailAndBundledRelation(bundle.RelationKeySyncError, domain.Int64(syncError))
	st.SetDetailAndBundledRelation(bundle.RelationKeySyncDate, domain.Int64(time.Now().Unix()))

	return sb.Apply(st, smartblock.KeepInternalFlags /* do not erase flags */)
}

func (u *syncStatusUpdater) tryUpdateFromFileBackupStatus(status domain.ObjectSyncStatus, syncError domain.SyncError, localDetails *domain.Details, details *domain.Details, spaceId string) (domain.ObjectSyncStatus, domain.SyncError) {
	if fileStatus, ok := details.TryFloat64(bundle.RelationKeyFileBackupStatus); ok {
		fStatus, fSyncError := getSyncStatusForFile(status, syncError, filesyncstatus.Status(int(fileStatus)))

		// Show oversized error for everyone
		if fSyncError == domain.SyncErrorOversized {
			return fStatus, fSyncError
		}

		// Show detailed sync status only for the current user
		if localDetails.GetString(bundle.RelationKeyCreator) == domain.NewParticipantId(spaceId, u.myIdentity) {
			return fStatus, fSyncError
		}
	}

	return status, syncError
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
	model.ObjectType_chatDeprecated: {},
	model.ObjectType_spaceView:      {},
	model.ObjectType_chatDerived:    {},
}

func (u *syncStatusUpdater) isLayoutSuitableForSyncRelations(details *domain.Details) bool {
	//nolint:gosec
	layout := model.ObjectTypeLayout(details.GetInt64(bundle.RelationKeyResolvedLayout))
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
