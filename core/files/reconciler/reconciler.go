package reconciler

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/filestorage"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

const CName = "core.files.reconciler"

var log = logging.Logger(CName).Desugar()

type Reconciler interface {
	app.ComponentRunnable

	FileObjectHook(id domain.FullID) func(applyInfo smartblock.ApplyInfo) error
}

type reconciler struct {
	objectStore  objectstore.ObjectStore
	fileSync     filesync.FileSync
	fileStorage  filestorage.FileStorage
	objectGetter cache.ObjectGetter

	rebindQueue           *persistentqueue.Queue[*queueItem]
	markAsReconciledQueue *persistentqueue.Queue[*queueItem]
}

type queueItem struct {
	ObjectId string
	FileId   domain.FullFileId
}

func makeQueueItem() *queueItem {
	return &queueItem{}
}

func (it *queueItem) Key() string {
	return it.ObjectId
}

func New() Reconciler {
	return &reconciler{}
}

func (r *reconciler) Init(a *app.App) error {
	r.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	r.fileSync = app.MustComponent[filesync.FileSync](a)
	r.fileStorage = app.MustComponent[filestorage.FileStorage](a)
	r.objectGetter = app.MustComponent[cache.ObjectGetter](a)

	r.fileSync.OnUploaded(r.markAsReconciled)

	dbProvider := app.MustComponent[datastore.Datastore](a)
	db, err := dbProvider.LocalStorage()
	if err != nil {
		return fmt.Errorf("get badger: %w", err)
	}
	r.rebindQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, []byte("queue/file_reconciler/rebind"), makeQueueItem), log, r.rebindHandler)
	r.markAsReconciledQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, []byte("queue/file_reconciler/mark"), makeQueueItem), log, r.markAsReconciledHandler)
	return nil
}

func (r *reconciler) Run(ctx context.Context) error {
	r.rebindQueue.Run()
	r.markAsReconciledQueue.Run()
	return r.reconcileRemoteStorage(ctx)
}

func (r *reconciler) FileObjectHook(id domain.FullID) func(applyInfo smartblock.ApplyInfo) error {
	return func(applyInfo smartblock.ApplyInfo) error {
		if !needToUpdateReconcilationStatus(applyInfo.State.Details()) {
			return nil
		}

		fileId := domain.FileId(pbtypes.GetString(applyInfo.State.Details(), bundle.RelationKeyFileId.String()))
		return r.rebindQueue.Add(&queueItem{ObjectId: id.ObjectID, FileId: domain.FullFileId{FileId: fileId, SpaceId: id.SpaceID}})
	}
}

func needToUpdateReconcilationStatus(details *types.Struct) bool {
	backupStatus := filesyncstatus.Status(pbtypes.GetInt64(details, bundle.RelationKeyFileBackupStatus.String()))
	if backupStatus != filesyncstatus.Synced {
		return false
	}

	reconcilationStatus := domain.ReconcilationStatus(pbtypes.GetInt64(details, bundle.RelationKeyFileReconcilationStatus.String()))
	return reconcilationStatus != domain.ReconcilationStatusDone
}

func (r *reconciler) rebindHandler(ctx context.Context, item *queueItem) (persistentqueue.Action, error) {
	err := r.fileSync.CancelDeletion(item.ObjectId, item.FileId)
	if err != nil {
		return persistentqueue.ActionRetry, fmt.Errorf("cancel deletion: %w", err)
	}

	log.Warn("add to queue", zap.String("objectId", item.ObjectId), zap.String("fileId", item.FileId.FileId.String()))
	err = r.fileSync.AddFile(item.ObjectId, item.FileId, false, false)
	if err != nil {
		return persistentqueue.ActionRetry, fmt.Errorf("upload file: %w", err)
	}

	return persistentqueue.ActionDone, nil
}

func (r *reconciler) markAsReconciledHandler(ctx context.Context, item *queueItem) (persistentqueue.Action, error) {
	fmt.Println("markAsReconciledHandler", item.ObjectId)
	return persistentqueue.ActionDone, r.markAsReconciled(item.ObjectId)
}

func (r *reconciler) markAsReconciled(fileObjectId string) error {
	return cache.Do(r.objectGetter, fileObjectId, func(sb smartblock.SmartBlock) (err error) {
		detailsSetter, ok := sb.(basic.DetailsSettable)
		if !ok {
			return fmt.Errorf("setting of details is not supported for %T", sb)
		}
		fmt.Println("UPDATE RECONC STATUS", fileObjectId)
		return detailsSetter.SetDetails(nil, []*model.Detail{
			{
				Key:   bundle.RelationKeyFileReconcilationStatus.String(),
				Value: pbtypes.Int64(int64(domain.ReconcilationStatusDone)),
			},
		}, true)
	})
}

func (r *reconciler) reconcileRemoteStorage(ctx context.Context) error {
	records, _, err := r.objectStore.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileId.String(),
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Bool(true),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query file objects: %w", err)
	}

	haveIds := map[domain.FileId]*types.Struct{}
	for _, rec := range records {
		fileId := domain.FileId(pbtypes.GetString(rec.Details, bundle.RelationKeyFileId.String()))
		if fileId.Valid() {
			haveIds[fileId] = rec.Details
		}
	}

	err = r.fileStorage.IterateFiles(ctx, func(fileId domain.FullFileId) {
		if details, ok := haveIds[fileId.FileId]; ok {
			if needToUpdateReconcilationStatus(details) {
				objectId := pbtypes.GetString(details, bundle.RelationKeyId.String())
				err := r.markAsReconciledQueue.Add(&queueItem{ObjectId: objectId})
				if err != nil {
					log.Error("add to mark as reconciled queue", zap.String("objectId", objectId), zap.Error(err))
				}
			}
		} else {
			log.Warn("file not found in local vault, enqueue deletion", zap.String("fileId", fileId.FileId.String()))
			err := r.fileSync.DeleteFile("", fileId)
			if err != nil {
				log.Error("add to deletion queue", zap.String("fileId", fileId.FileId.String()), zap.Error(err))
			}
		}
	})
	if err != nil {
		return fmt.Errorf("iterate files: %w", err)
	}

	// TODO Unbind files not found in local vault
	// TODO Create queue for files that appeared in local vault after initial deletion
	// TODO Add hook to file object to add it to reconcilation queue
	// TODO Reconcilation queue handler: pass noCache context and rebind file
	// TODO After restart add missing files to reconcilation queue
	return nil
}

func (r *reconciler) Name() string {
	return CName
}

func (r *reconciler) Close(ctx context.Context) error {
	return r.rebindQueue.Close()
}
