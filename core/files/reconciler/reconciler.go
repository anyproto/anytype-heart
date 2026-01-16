package reconciler

import (
	"context"
	"errors"
	"fmt"
	"sync"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/keyvaluestore"
	"github.com/anyproto/anytype-heart/util/persistentqueue"
)

const (
	CName = "core.files.reconciler"
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

	r.fileSync.OnStatusUpdated(r.markAsReconciled)

	provider := app.MustComponent[anystoreprovider.Provider](a)
	db := provider.GetCommonDb()

	var err error
	r.deletedFiles, err = keyvaluestore.New(db, "file_reconciler/deleted_files", func(_ struct{}) ([]byte, error) {
		return []byte(""), nil
	}, func(data []byte) (struct{}, error) {
		return struct{}{}, nil
	})

	rebindQueueStore, err := persistentqueue.NewAnystoreStorage(db, "queue/file_reconciler/rebind", makeQueueItem)
	if err != nil {
		return fmt.Errorf("init rebindQueueStore: %w", err)
	}
	r.rebindQueue = persistentqueue.New(rebindQueueStore, log, r.rebindHandler, nil)

	r.isStartedStore = keyvaluestore.NewJsonFromCollection[bool](provider.GetSystemCollection())

	return nil
}

func (r *reconciler) Run(ctx context.Context) error {
	isStarted, err := r.isStartedStore.Get(context.Background(), anystoreprovider.SystemKeys.FileReconcilerStarted())
	if err != nil && !errors.Is(err, anystore.ErrDocNotFound) {
		log.Error("get isStarted", zap.Error(err))
	}
	r.lock.Lock()
	r.isStarted = isStarted
	r.lock.Unlock()

	r.rebindQueue.Run()
	return nil
}

func (r *reconciler) Start(ctx context.Context) error {
	err := r.isStartedStore.Set(context.Background(), anystoreprovider.SystemKeys.FileReconcilerStarted(), true)
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
			fileId := domain.FileId(applyInfo.State.Details().GetString(bundle.RelationKeyFileId))
			return r.rebindQueue.Add(&queueItem{ObjectId: id.ObjectID, FileId: domain.FullFileId{FileId: fileId, SpaceId: id.SpaceID}})
		}
		return nil
	}
}

func (r *reconciler) needToRebind(details *domain.Details) (bool, error) {
	if details.GetBool(bundle.RelationKeyIsDeleted) {
		return false, nil
	}
	backupStatus := filesyncstatus.Status(details.GetInt64(bundle.RelationKeyFileBackupStatus))
	// It makes no sense to rebind file that hasn't been uploaded yet, because this file could be uploading
	// by another client. When another client will upload this file, FileObjectHook will be called with FileBackupStatus == Synced
	if backupStatus != filesyncstatus.Synced {
		return false, nil
	}
	fileId := domain.FileId(details.GetString(bundle.RelationKeyFileId))
	return r.deletedFiles.Has(context.Background(), fileId.String())
}

func (r *reconciler) rebindHandler(ctx context.Context, item *queueItem) (persistentqueue.Action, error) {
	log.Warn("add to queue", zap.String("objectId", item.ObjectId), zap.String("fileId", item.FileId.FileId.String()))
	req := filesync.AddFileRequest{
		FileObjectId:   item.ObjectId,
		FileId:         item.FileId,
		UploadedByUser: false,
		Imported:       false,
	}
	err := r.fileSync.AddFile(req)
	if err != nil {
		return persistentqueue.ActionRetry, fmt.Errorf("upload file: %w", err)
	}

	return persistentqueue.ActionDone, nil
}

func (r *reconciler) markAsReconciled(fileObjectId string, fileId domain.FullFileId, status filesyncstatus.Status) error {
	if !r.isRunning() {
		return nil
	}
	if status == filesyncstatus.Synced {
		return r.deletedFiles.Delete(context.Background(), fileId.FileId.String())
	}
	return nil
}

func (r *reconciler) reconcileRemoteStorage(ctx context.Context) error {
	records, err := r.objectStore.QueryCrossSpace(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyFileId,
				Condition:   model.BlockContentDataviewFilter_NotEmpty,
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("query file objects: %w", err)
	}

	haveIds := map[domain.FileId]struct{}{}
	for _, rec := range records {
		fileId := domain.FileId(rec.Details.GetString(bundle.RelationKeyFileId))
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
			err = r.deletedFiles.Set(context.Background(), fileId.FileId.String(), struct{}{})
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
