package reconciler

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

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
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

const (
	CName             = "core.files.reconciler"
	isStartedStoreKey = "value"
)

var log = logging.Logger(CName).Desugar()

type Reconciler interface {
	app.ComponentRunnable

	Start(ctx context.Context) error
	FileObjectHook(id domain.FullID) func(applyInfo smartblock.ApplyInfo) error
}

type reconciler struct {
	objectStore objectstore.ObjectStore
	fileSync    filesync.FileSync
	fileStorage filestorage.FileStorage

	lock      sync.Mutex
	isStarted bool

	isStartedStore keyvaluestore.Store[bool]
	deletedFiles   keyvaluestore.Store[struct{}]
	rebindQueue    *persistentqueue.Queue[*queueItem]
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

	r.fileSync.OnUploaded(r.markAsReconciled)

	dbProvider := app.MustComponent[datastore.Datastore](a)
	db, err := dbProvider.LocalStorage()
	if err != nil {
		return fmt.Errorf("get badger: %w", err)
	}

	r.isStartedStore = keyvaluestore.NewJson[bool](db, []byte("file_reconciler/is_started"))
	r.deletedFiles = keyvaluestore.New(db, []byte("file_reconciler/deleted_files"), func(_ struct{}) ([]byte, error) {
		return []byte(""), nil
	}, func(data []byte) (struct{}, error) {
		return struct{}{}, nil
	})
	r.rebindQueue = persistentqueue.New(persistentqueue.NewBadgerStorage(db, []byte("queue/file_reconciler/rebind"), makeQueueItem), log, r.rebindHandler)
	return nil
}

func (r *reconciler) Run(ctx context.Context) error {
	isStarted, err := r.isStartedStore.Get(isStartedStoreKey)
	if err != nil && !errors.Is(err, keyvaluestore.ErrNotFound) {
		log.Error("get isStarted", zap.Error(err))
	}
	r.lock.Lock()
	r.isStarted = isStarted
	r.lock.Unlock()

	r.rebindQueue.Run()
	return nil
}

func (r *reconciler) Start(ctx context.Context) error {
	err := r.isStartedStore.Set(isStartedStoreKey, true)
	if err != nil {
		return fmt.Errorf("set isStarted: %w", err)
	}
	r.lock.Lock()
	r.isStarted = true
	r.lock.Unlock()

	return r.reconcileRemoteStorage(ctx)
}

func (r *reconciler) isRunning() bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	return r.isStarted
}

func (r *reconciler) FileObjectHook(id domain.FullID) func(applyInfo smartblock.ApplyInfo) error {
	return func(applyInfo smartblock.ApplyInfo) error {
		if !r.isRunning() {
			return nil
		}
		ok, err := r.needToRebind(applyInfo.State.Details())
		if err != nil {
			return fmt.Errorf("need to rebind: %w", err)
		}
		if ok {
			fileId := domain.FileId(pbtypes.GetString(applyInfo.State.Details(), bundle.RelationKeyFileId.String()))
			return r.rebindQueue.Add(&queueItem{ObjectId: id.ObjectID, FileId: domain.FullFileId{FileId: fileId, SpaceId: id.SpaceID}})
		}
		return nil
	}
}

func (r *reconciler) needToRebind(details *types.Struct) (bool, error) {
	if pbtypes.GetBool(details, bundle.RelationKeyIsDeleted.String()) {
		return false, nil
	}
	backupStatus := filesyncstatus.Status(pbtypes.GetInt64(details, bundle.RelationKeyFileBackupStatus.String()))
	// It makes no sense to rebind file that hasn't been uploaded yet, because this file could be uploading
	// by another client. When another client will upload this file, FileObjectHook will be called with FileBackupStatus == Synced
	if backupStatus != filesyncstatus.Synced {
		return false, nil
	}
	fileId := domain.FileId(pbtypes.GetString(details, bundle.RelationKeyFileId.String()))
	return r.deletedFiles.Has(fileId.String())
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

func (r *reconciler) markAsReconciled(fileObjectId string, fileId domain.FullFileId) error {
	if !r.isRunning() {
		return nil
	}
	return r.deletedFiles.Delete(fileId.FileId.String())
}

func (r *reconciler) reconcileRemoteStorage(ctx context.Context) error {
	records, err := r.objectStore.Query(database.Query{
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

	haveIds := map[domain.FileId]struct{}{}
	for _, rec := range records {
		fileId := domain.FileId(pbtypes.GetString(rec.Details, bundle.RelationKeyFileId.String()))
		if fileId.Valid() {
			haveIds[fileId] = struct{}{}
		}
	}

	err = r.fileStorage.IterateFiles(ctx, func(fileId domain.FullFileId) {
		if _, ok := haveIds[fileId.FileId]; !ok {
			log.Warn("file not found in local vault, enqueue deletion", zap.String("fileId", fileId.FileId.String()))
			err := r.fileSync.DeleteFile("", fileId)
			if err != nil {
				log.Error("add to deletion queue", zap.String("fileId", fileId.FileId.String()), zap.Error(err))
			}
			err = r.deletedFiles.Set(fileId.FileId.String(), struct{}{})
			if err != nil {
				log.Error("add to deleted files", zap.String("fileId", fileId.FileId.String()), zap.Error(err))
			}
		}
	})
	if err != nil {
		return fmt.Errorf("iterate files: %w", err)
	}
	return nil
}

func (r *reconciler) Name() string {
	return CName
}

func (r *reconciler) Close(ctx context.Context) error {
	return r.rebindQueue.Close()
}
