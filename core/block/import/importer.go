package importer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	creator "github.com/anyproto/anytype-heart/core/block/import/common/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/import/common/objectid"
	"github.com/anyproto/anytype-heart/core/block/import/common/syncer"
	"github.com/anyproto/anytype-heart/core/block/import/common/workerpool"
	"github.com/anyproto/anytype-heart/core/block/import/csv"
	"github.com/anyproto/anytype-heart/core/block/import/html"
	"github.com/anyproto/anytype-heart/core/block/import/markdown"
	"github.com/anyproto/anytype-heart/core/block/import/notion"
	pbc "github.com/anyproto/anytype-heart/core/block/import/pb"
	"github.com/anyproto/anytype-heart/core/block/import/txt"
	"github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/core/notifications"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/metrics/anymetry"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/conc"
)

var log = logging.Logger("import")

const CName = "importer"

const workerPoolSize = 10

type Import struct {
	converters          map[string]common.Converter
	s                   *block.Service
	oc                  creator.Service
	idProvider          objectid.IdAndKeyProvider
	tempDirProvider     core.TempDirProvider
	fileSync            filesync.FileSync
	notificationService notifications.Notifications
	eventSender         event.Sender
	objectStore         objectstore.ObjectStore

	importCtx       context.Context
	importCtxCancel context.CancelFunc
	spaceService    space.Service
}

func New() Importer {
	return &Import{
		converters: make(map[string]common.Converter, 0),
	}
}

func (i *Import) Init(a *app.App) (err error) {
	i.s = app.MustComponent[*block.Service](a)
	accountService := app.MustComponent[account.Service](a)
	spaceService := app.MustComponent[space.Service](a)
	i.spaceService = spaceService
	col := app.MustComponent[*collection.Service](a)
	i.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	converters := []common.Converter{
		markdown.New(i.tempDirProvider, col),
		notion.New(col),
		pbc.New(col, accountService, i.tempDirProvider),
		web.NewConverter(),
		html.New(col, i.tempDirProvider),
		txt.New(col),
		csv.New(col),
	}
	for _, c := range converters {
		i.converters[c.Name()] = c
	}
	// temporary, until we don't have specific logic for obsidian import
	i.converters[model.Import_Obsidian.String()] = i.converters[model.Import_Markdown.String()]
	i.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	fileObjectService := app.MustComponent[fileobject.Service](a)
	i.idProvider = objectid.NewIDProvider(i.objectStore, spaceService, i.s, fileObjectService)
	factory := syncer.New(syncer.NewFileSyncer(i.s, fileObjectService), syncer.NewBookmarkSyncer(i.s), syncer.NewIconSyncer(i.s, fileObjectService))
	relationSyncer := syncer.NewFileRelationSyncer(i.s, fileObjectService)
	objectCreator := app.MustComponent[objectcreator.Service](a)
	detailsService := app.MustComponent[detailservice.Service](a)
	i.oc = creator.New(detailsService, factory, i.objectStore, relationSyncer, spaceService, objectCreator, i.s)
	i.fileSync = app.MustComponent[filesync.FileSync](a)
	i.notificationService = app.MustComponent[notifications.Notifications](a)
	i.eventSender = app.MustComponent[event.Sender](a)

	i.importCtx, i.importCtxCancel = context.WithCancel(context.Background())
	return nil
}

func (i *Import) Run(ctx context.Context) (err error) {
	return
}

func (i *Import) Close(ctx context.Context) (err error) {
	if i.importCtxCancel != nil {
		i.importCtxCancel()
	}
	return
}

// Import get snapshots from converter or external api and create smartblocks from them
func (i *Import) Import(ctx context.Context, importRequest *ImportRequest) *ImportResponse {
	if importRequest.IsSync {
		return i.importObjects(ctx, importRequest)
	}
	conc.Go(func() {
		res := i.importObjects(i.importCtx, importRequest)
		if res.Err != nil {
			log.Errorf("import from %s failed with error: %s", importRequest.Type.String(), res.Err)
		}
	})
	return nil
}

func (i *Import) importObjects(ctx context.Context, importRequest *ImportRequest) *ImportResponse {
	if importRequest.SpaceId == "" {
		return &ImportResponse{
			RootCollectionId: "",
			ProcessId:        "",
			ObjectsCount:     0,
			Err:              fmt.Errorf("spaceId is empty"),
		}
	}
	var (
		res           = &ImportResponse{}
		importId      = uuid.New().String()
		isNewProgress = false
		widgetLayout  model.BlockContentWidgetLayout
	)
	if importRequest.Progress == nil {
		i.setupProgressBar(importRequest)
		isNewProgress = true
	}
	defer func() {
		i.onImportFinish(res, importRequest, importId)
	}()
	if i.s != nil && !importRequest.GetNoProgress() && isNewProgress {
		err := i.s.ProcessAdd(importRequest.Progress)
		if err != nil {
			return &ImportResponse{Err: fmt.Errorf("failed to add process")}
		}
	}
	i.recordEvent(&metrics.ImportStartedEvent{ID: importId, ImportType: importRequest.Type.String()})
	res.Err = fmt.Errorf("unknown import type %s", importRequest.Type)

	if c, ok := i.converters[importRequest.Type.String()]; ok {
		res.RootCollectionId, widgetLayout, res.ObjectsCount, res.Err = i.importFromBuiltinConverter(ctx, importRequest, c)
	}
	if importRequest.Type == model.Import_External {
		res.ObjectsCount, res.Err = i.importFromExternalSource(ctx, importRequest)
	}
	res.ProcessId = importRequest.Progress.Id()
	res.RootWidgetLayout = widgetLayout

	return res
}

func (i *Import) onImportFinish(res *ImportResponse, req *ImportRequest, importId string) {
	i.finishImportProcess(res.Err, req)
	i.sendFileEvents(res.Err)
	if res.RootCollectionId != "" {
		i.addRootCollectionWidget(res, req)
	}

	i.recordEvent(&metrics.ImportFinishedEvent{ID: importId, ImportType: req.Type.String()})
	i.sendImportFinishEventToClient(res.RootCollectionId, req.IsSync, res.ObjectsCount, req.Type)
}

func (i *Import) typeWidgetCreation(req *ImportRequest, typeKeys []domain.TypeKey) {
	err := i.s.CreateTypeWidgetsIfMissing(context.Background(), req.SpaceId, typeKeys, true)
	if err != nil {
		log.Errorf("failed to create widget from root collection, error: %s", err.Error())
	}
}

func (i *Import) addRootCollectionWidget(res *ImportResponse, req *ImportRequest) {
	spc, err := i.spaceService.Get(i.importCtx, req.SpaceId)
	if err != nil {
		log.Errorf("failed to create widget from root collection, error: %s", err.Error())
	} else {
		_, err = i.s.CreateWidgetBlock(nil, &pb.RpcBlockCreateWidgetRequest{
			ContextId:    spc.DerivedIDs().Widgets,
			WidgetLayout: res.RootWidgetLayout,
			Block: &model.Block{
				Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
					TargetBlockId: res.RootCollectionId,
				}},
			},
		}, true)
		if err != nil {
			log.Errorf("failed to create widget from root collection, error: %s", err.Error())
		}
	}
}

func (i *Import) sendFileEvents(returnedErr error) {
	if returnedErr == nil {
		i.fileSync.SendImportEvents()
	}
	i.fileSync.ClearImportEvents()
}

func (i *Import) importFromBuiltinConverter(ctx context.Context, req *ImportRequest, c common.Converter) (string, model.BlockContentWidgetLayout, int64, error) {
	allErrors := common.NewError(req.Mode)
	res, err := c.GetSnapshots(ctx, req.RpcObjectImportRequest, req.Progress)
	if !err.IsEmpty() {
		resultErr := err.GetResultError(req.Type)
		if shouldReturnError(resultErr, res, req.RpcObjectImportRequest) {
			return "", 0, 0, resultErr
		}
		allErrors.Merge(err)
	}
	if res == nil {
		return "", 0, 0, fmt.Errorf("source path doesn't contain %s resources to import", req.Type)
	}

	if len(res.Snapshots) == 0 {
		return "", 0, 0, fmt.Errorf("source path doesn't contain %s resources to import", req.Type)
	}

	i.typeWidgetCreation(req, res.TypesCreated)
	details, rootCollectionID := i.createObjects(ctx, res, req.Progress, req.RpcObjectImportRequest, allErrors, req.Origin)
	resultErr := allErrors.GetResultError(req.Type)
	if resultErr != nil {
		rootCollectionID = ""
	}

	return rootCollectionID, res.RootObjectWidgetType, i.getObjectCount(details, rootCollectionID), resultErr
}

func (i *Import) getObjectCount(details map[string]*domain.Details, rootCollectionID string) int64 {
	objectsCount := int64(len(details))
	if rootCollectionID != "" && objectsCount > 0 {
		objectsCount-- // exclude root collection object from counter
	}
	return objectsCount
}

func (i *Import) importFromExternalSource(ctx context.Context, req *ImportRequest) (int64, error) {
	allErrors := common.NewError(req.Mode)
	if req.Snapshots != nil {
		sn := make([]*common.Snapshot, len(req.Snapshots))
		for i, s := range req.Snapshots {
			sn[i] = &common.Snapshot{
				Id: s.GetId(),
				Snapshot: &common.SnapshotModel{
					Data: common.NewStateSnapshotFromProto(s.Snapshot),
				},
			}
		}
		res := &common.Response{
			Snapshots: sn,
		}

		originImport := objectorigin.Import(model.Import_External)
		details, _ := i.createObjects(ctx, res, req.Progress, req.RpcObjectImportRequest, allErrors, originImport)
		if !allErrors.IsEmpty() {
			return 0, allErrors.GetResultError(req.Type)
		}
		return int64(len(details)), nil
	}
	return 0, common.ErrNoSnapshotToImport
}

func (i *Import) finishImportProcess(returnedErr error, req *ImportRequest) {
	if notificationProgress, ok := req.Progress.(process.Notificationable); ok {
		notificationProgress.FinishWithNotification(i.provideNotification(returnedErr, req.Progress, req), returnedErr)
	} else {
		req.Progress.Finish(returnedErr)
	}
}

func (i *Import) provideNotification(returnedErr error, progress process.Progress, req *ImportRequest) *model.Notification {
	return &model.Notification{
		Id:      uuid.New().String(),
		Status:  model.Notification_Created,
		IsLocal: true,
		Space:   req.SpaceId,
		Payload: &model.NotificationPayloadOfImport{Import: &model.NotificationImport{
			ProcessId:  progress.Id(),
			ErrorCode:  common.GetImportNotificationErrorCode(returnedErr),
			ImportType: req.Type,
			SpaceId:    req.SpaceId,
		}},
	}
}

func shouldReturnError(e error, res *common.Response, req *pb.RpcObjectImportRequest) bool {
	return (e != nil && req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS) ||
		errors.Is(e, common.ErrNotionServerExceedRateLimit) || errors.Is(e, common.ErrCsvLimitExceeded) ||
		(common.IsNoObjectError(e) && (res == nil || len(res.Snapshots) == 0)) || // return error only if we don't have object to import
		errors.Is(e, common.ErrCancel)
}

func (i *Import) setupProgressBar(req *ImportRequest) {
	var progressBarType pb.IsModelProcessMessage = &pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}}
	if req.IsMigration {
		progressBarType = &pb.ModelProcessMessageOfMigration{Migration: &pb.ModelProcessMigration{}}
	}
	var progress process.Progress
	if req.GetNoProgress() {
		progress = process.NewNoOp()
	} else {
		progress = process.NewProgress(progressBarType)
		if req.SendNotification {
			progress = process.NewNotificationProcess(progressBarType, i.notificationService)
		}
	}
	req.Progress = progress
}

func (i *Import) Name() string {
	return CName
}

// ListImports return all registered import types
func (i *Import) ListImports(_ *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error) {
	res := make([]*pb.RpcObjectImportListImportResponse, len(i.converters))
	var idx int
	for _, c := range i.converters {
		res[idx] = &pb.RpcObjectImportListImportResponse{Type: convertType(c.Name())}
		idx++
	}
	return res, nil
}

// ValidateNotionToken return all registered import types
func (i *Import) ValidateNotionToken(
	ctx context.Context, req *pb.RpcObjectImportNotionValidateTokenRequest,
) (pb.RpcObjectImportNotionValidateTokenResponseErrorCode, error) {
	tv := notion.NewTokenValidator()
	return tv.Validate(ctx, req.GetToken())
}

func (i *Import) ImportWeb(ctx context.Context, req *ImportRequest) (string, *domain.Details, error) {
	if req.Progress == nil {
		i.setupProgressBar(req)
	}
	defer req.Progress.Finish(nil)
	if i.s != nil {
		err := i.s.ProcessAdd(req.Progress)
		if err != nil {
			return "", nil, fmt.Errorf("failed to add process")
		}
	}
	allErrors := common.NewError(0)

	req.Progress.SetProgressMessage("Parse url")
	w := i.converters[web.Name]
	res, err := w.GetSnapshots(ctx, req.RpcObjectImportRequest, req.Progress)

	if err != nil {
		return "", nil, err.Error()
	}
	if res.Snapshots == nil || len(res.Snapshots) == 0 {
		return "", nil, fmt.Errorf("snpashots are empty")
	}

	req.Progress.SetProgressMessage("Create objects")
	details, _ := i.createObjects(ctx, res, req.Progress, req.RpcObjectImportRequest, allErrors, objectorigin.None())
	if !allErrors.IsEmpty() {
		return "", nil, fmt.Errorf("couldn't create objects")
	}
	return res.Snapshots[0].Id, details[res.Snapshots[0].Id], nil
}

func (i *Import) createObjects(ctx context.Context,
	res *common.Response,
	progress process.Progress,
	req *pb.RpcObjectImportRequest,
	allErrors *common.ConvertError,
	origin objectorigin.ObjectOrigin,
) (map[string]*domain.Details, string) {
	oldIDToNew, createPayloads, err := i.getIDForAllObjects(ctx, res, allErrors, req, origin)
	if err != nil {
		return nil, ""
	}
	numWorkers := workerPoolSize
	if len(res.Snapshots) < workerPoolSize {
		numWorkers = 1
	}
	do := creator.NewDataObject(ctx, oldIDToNew, createPayloads, origin, req.SpaceId)
	pool := workerpool.NewPool(numWorkers)
	progress.SetProgressMessage("Create objects")
	go i.addWork(res, pool)
	go pool.Start(do)
	details := i.readResultFromPool(pool, req.Mode, allErrors, progress)
	return details, oldIDToNew[res.RootObjectID]
}

func (i *Import) getIDForAllObjects(ctx context.Context,
	res *common.Response,
	allErrors *common.ConvertError,
	req *pb.RpcObjectImportRequest,
	origin objectorigin.ObjectOrigin,
) (map[string]string, map[string]treestorage.TreeStorageCreatePayload, error) {
	relationOptions := make([]*common.Snapshot, 0)
	oldIDToNew := make(map[string]string, len(res.Snapshots))
	createPayloads := make(map[string]treestorage.TreeStorageCreatePayload, len(res.Snapshots))
	for _, snapshot := range res.Snapshots {
		// we will get id of relation options after we figure out according relations keys
		if lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyRelationOption.String()) {
			relationOptions = append(relationOptions, snapshot)
			continue
		}
		err := i.getObjectID(ctx, req.SpaceId, snapshot, createPayloads, oldIDToNew, req.UpdateExistingObjects, origin)
		if err != nil {
			allErrors.Add(err)
			if req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
				return nil, nil, err
			}
			log.With(zap.String("object name", snapshot.Id)).Error(err)
		}
	}
	for _, option := range relationOptions {
		i.replaceRelationKeyWithNew(option, oldIDToNew)
		err := i.getObjectID(ctx, req.SpaceId, option, createPayloads, oldIDToNew, req.UpdateExistingObjects, origin)
		if err != nil {
			allErrors.Add(err)
			if req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
				return nil, nil, err
			}
			log.With(zap.String("object name", option.Id)).Error(err)
		}
	}
	return oldIDToNew, createPayloads, nil
}

func (i *Import) replaceRelationKeyWithNew(option *common.Snapshot, oldIDToNew map[string]string) {
	if option.Snapshot.Data.Details == nil || option.Snapshot.Data.Details.Len() == 0 {
		return
	}
	key := option.Snapshot.Data.Details.GetString(bundle.RelationKeyRelationKey)
	if newRelationID, ok := oldIDToNew[key]; ok {
		key = strings.TrimPrefix(newRelationID, addr.RelationKeyToIdPrefix)
	}
	option.Snapshot.Data.Details.SetString(bundle.RelationKeyRelationKey, key)
}

func (i *Import) getObjectID(
	ctx context.Context,
	spaceID string,
	snapshot *common.Snapshot,
	createPayloads map[string]treestorage.TreeStorageCreatePayload,
	oldIDToNew map[string]string,
	updateExisting bool,
	origin objectorigin.ObjectOrigin,
) error {

	// Preload file keys
	for _, fileKeys := range snapshot.Snapshot.FileKeys {
		err := i.objectStore.AddFileKeys(domain.FileEncryptionKeys{
			FileId:         domain.FileId(fileKeys.Hash),
			EncryptionKeys: fileKeys.Keys,
		})
		if err != nil {
			return fmt.Errorf("add file keys: %w", err)
		}
	}
	if fileInfo := snapshot.Snapshot.Data.FileInfo; fileInfo != nil {
		keys := make(map[string]string, len(fileInfo.EncryptionKeys))
		for _, key := range fileInfo.EncryptionKeys {
			keys[key.Path] = key.Key
		}
		err := i.objectStore.AddFileKeys(domain.FileEncryptionKeys{
			FileId:         domain.FileId(fileInfo.FileId),
			EncryptionKeys: keys,
		})
		if err != nil {
			return fmt.Errorf("add file keys from file info: %w", err)
		}
	}

	var (
		id      string
		payload treestorage.TreeStorageCreatePayload
	)
	id, payload, err := i.idProvider.GetIDAndPayload(ctx, spaceID, snapshot, time.Now(), updateExisting, origin)
	if err != nil {
		return err
	}
	oldIDToNew[snapshot.Id] = id
	var isBundled bool
	switch snapshot.Snapshot.SbType {
	case smartblock.SmartBlockTypeObjectType:
		isBundled = bundle.HasObjectTypeByKey(domain.TypeKey(snapshot.Snapshot.Data.Key))
	case smartblock.SmartBlockTypeRelation:
		isBundled = bundle.HasRelation(domain.RelationKey(snapshot.Snapshot.Data.Key))
	}
	// bundled types will be created and then updated, cause they can be installed asynchronously
	if payload.RootRawChange != nil && !isBundled {
		createPayloads[id] = payload
	}
	return i.extractInternalKey(snapshot, oldIDToNew)
}

func (i *Import) extractInternalKey(snapshot *common.Snapshot, oldIDToNew map[string]string) error {
	newUniqueKey := i.idProvider.GetInternalKey(snapshot.Snapshot.SbType)
	if newUniqueKey != "" {
		oldUniqueKey := snapshot.Snapshot.Data.Details.GetString(bundle.RelationKeyUniqueKey)
		if oldUniqueKey == "" {
			oldUniqueKey = snapshot.Snapshot.Data.Key
		}
		if oldUniqueKey != "" {
			oldIDToNew[oldUniqueKey] = newUniqueKey
		}
	}
	return nil
}

func (i *Import) addWork(res *common.Response, pool *workerpool.WorkerPool) {
	for _, snapshot := range res.Snapshots {
		t := creator.NewTask(snapshot, i.oc)
		stop := pool.AddWork(t)
		if stop {
			break
		}
	}
	pool.CloseTask()
}

func (i *Import) readResultFromPool(pool *workerpool.WorkerPool,
	mode pb.RpcObjectImportRequestMode,
	allErrors *common.ConvertError,
	progress process.Progress,
) map[string]*domain.Details {
	details := make(map[string]*domain.Details, 0)
	for r := range pool.Results() {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(fmt.Errorf("%w: %s", common.ErrCancel, err.Error()))
			pool.Stop()
			return nil
		}
		res := r.(*creator.Result)
		if res.Err != nil {
			allErrors.Add(res.Err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				pool.Stop()
				return nil
			}
		}
		details[res.NewID] = res.Details
	}
	return details
}

func (i *Import) recordEvent(event anymetry.Event) {
	metrics.Service.Send(event)
}

func (i *Import) sendImportFinishEventToClient(rootCollectionID string, isSync bool, objectsCount int64, importType model.ImportType) {
	if isSync {
		return
	}
	i.eventSender.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfImportFinish{
		ImportFinish: &pb.EventImportFinish{
			RootCollectionID: rootCollectionID,
			ObjectsCount:     objectsCount,
			ImportType:       importType,
		},
	}))
}

func convertType(cType string) pb.RpcObjectImportListImportResponseType {
	return pb.RpcObjectImportListImportResponseType(pb.RpcObjectImportListImportResponseType_value[cType])
}
