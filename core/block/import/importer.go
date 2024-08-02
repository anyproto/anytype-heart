package importer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/collection"
	creator "github.com/anyproto/anytype-heart/core/block/import/common/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/import/common/objectid"
	"github.com/anyproto/anytype-heart/core/block/import/common/syncer"
	types2 "github.com/anyproto/anytype-heart/core/block/import/common/types"
	"github.com/anyproto/anytype-heart/core/block/import/common/workerpool"
	"github.com/anyproto/anytype-heart/core/block/import/csv"
	"github.com/anyproto/anytype-heart/core/block/import/html"
	"github.com/anyproto/anytype-heart/core/block/import/markdown"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/import/notion"
	pbc "github.com/anyproto/anytype-heart/core/block/import/pb"
	"github.com/anyproto/anytype-heart/core/block/import/txt"
	"github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/metrics/anymetry"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("import")

const CName = "importer"

const workerPoolSize = 10

type Import struct {
	converters      map[string]types2.Converter
	s               *block.Service
	oc              creator.Service
	idProvider      objectid.IdAndKeyProvider
	tempDirProvider core.TempDirProvider
	fileStore       filestore.FileStore
	fileSync        filesync.FileSync
	sync.Mutex
}

func New() Importer {
	return &Import{
		converters: make(map[string]types2.Converter, 0),
	}
}

func (i *Import) Init(a *app.App) (err error) {
	i.s = app.MustComponent[*block.Service](a)
	accountService := app.MustComponent[account.Service](a)
	spaceService := app.MustComponent[space.Service](a)
	col := app.MustComponent[*collection.Service](a)
	i.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	converters := []types2.Converter{
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
	store := app.MustComponent[objectstore.ObjectStore](a)
	i.fileStore = app.MustComponent[filestore.FileStore](a)
	fileObjectService := app.MustComponent[fileobject.Service](a)
	i.idProvider = objectid.NewIDProvider(store, spaceService, i.s, i.fileStore, fileObjectService)
	factory := syncer.New(syncer.NewFileSyncer(i.s, fileObjectService), syncer.NewBookmarkSyncer(i.s), syncer.NewIconSyncer(i.s, fileObjectService))
	relationSyncer := syncer.NewFileRelationSyncer(i.s, fileObjectService)
	objectCreator := app.MustComponent[objectcreator.Service](a)
	i.oc = creator.New(i.s, factory, store, relationSyncer, spaceService, objectCreator)
	i.fileSync = app.MustComponent[filesync.FileSync](a)
	return nil
}
func (i *Import) ImportSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest) ([]*types2.Snapshot, error) {
	if c, ok := i.converters[req.Type.String()]; ok {
		snapshots, err := c.GetSnapshots(ctx, req, nil)
		if err != nil && err.Error() != nil {
			return nil, err.Error()
		}
		for _, snapshot := range snapshots.Snapshots {
			snapshot.Snapshot.Data.Blocks = anymark.AddRootBlock(snapshot.Snapshot.Data.Blocks, snapshot.Id)
		}
		return snapshots.Snapshots, nil
	}
	return nil, fmt.Errorf("no pb import")
}

// Import get snapshots from converter or external api and create smartblocks from them
func (i *Import) Import(ctx context.Context,
	req *pb.RpcObjectImportRequest,
	origin objectorigin.ObjectOrigin,
	progress process.Progress,
) *ImportResponse {
	if req.SpaceId == "" {
		return &ImportResponse{
			RootCollectionId: "",
			ProcessId:        "",
			ObjectsCount:     0,
			Err:              fmt.Errorf("spaceId is empty"),
		}
	}
	i.Lock()
	defer i.Unlock()
	isNewProgress := false
	if progress == nil {
		progress = i.setupProgressBar(req)
		isNewProgress = true
	}
	var (
		returnedErr error
		importId    = uuid.New().String()
	)
	defer func() {
		i.finishImportProcess(returnedErr, progress)
		i.sendFileEvents(returnedErr)
		i.recordEvent(&metrics.ImportFinishedEvent{ID: importId, ImportType: req.Type.String()})
	}()
	if i.s != nil && !req.GetNoProgress() && isNewProgress {
		i.s.ProcessAdd(progress)
	}
	i.recordEvent(&metrics.ImportStartedEvent{ID: importId, ImportType: req.Type.String()})
	var (
		rootCollectionId string
		objectsCount     int64
	)
	returnedErr = fmt.Errorf("unknown import type %s", req.Type)
	if c, ok := i.converters[req.Type.String()]; ok {
		rootCollectionId, objectsCount, returnedErr = i.importFromBuiltinConverter(ctx, req, c, progress, origin)
	}
	if req.Type == model.Import_External {
		objectsCount, returnedErr = i.importFromExternalSource(ctx, req, progress)
	}
	return &ImportResponse{
		RootCollectionId: rootCollectionId,
		ProcessId:        progress.Id(),
		ObjectsCount:     objectsCount,
		Err:              returnedErr,
	}
}

func (i *Import) sendFileEvents(returnedErr error) {
	if returnedErr == nil {
		i.fileSync.SendImportEvents()
	}
	i.fileSync.ClearImportEvents()
}

func (i *Import) importFromBuiltinConverter(ctx context.Context,
	req *pb.RpcObjectImportRequest,
	c types2.Converter,
	progress process.Progress,
	origin objectorigin.ObjectOrigin,
) (string, int64, error) {
	allErrors := types2.NewError(req.Mode)
	res, err := c.GetSnapshots(ctx, req, progress)
	if !err.IsEmpty() {
		resultErr := err.GetResultError(req.Type)
		if shouldReturnError(resultErr, res, req) {
			return "", 0, resultErr
		}
		allErrors.Merge(err)
	}
	if res == nil {
		return "", 0, fmt.Errorf("source path doesn't contain %s resources to import", req.Type)
	}

	if len(res.Snapshots) == 0 {
		return "", 0, fmt.Errorf("source path doesn't contain %s resources to import", req.Type)
	}

	details, rootCollectionID := i.createObjects(ctx, res, progress, req, allErrors, origin)
	resultErr := allErrors.GetResultError(req.Type)
	if resultErr != nil {
		rootCollectionID = ""
	}
	return rootCollectionID, i.getObjectCount(details, rootCollectionID), resultErr
}

func (i *Import) getObjectCount(details map[string]*types.Struct, rootCollectionID string) int64 {
	objectsCount := int64(len(details))
	if rootCollectionID != "" && objectsCount > 0 {
		objectsCount-- // exclude root collection object from counter
	}
	return objectsCount
}

func (i *Import) importFromExternalSource(ctx context.Context,
	req *pb.RpcObjectImportRequest,
	progress process.Progress,
) (int64, error) {
	allErrors := types2.NewError(req.Mode)
	if req.Snapshots != nil {
		sn := make([]*types2.Snapshot, len(req.Snapshots))
		for i, s := range req.Snapshots {
			sn[i] = &types2.Snapshot{
				Id:       s.GetId(),
				Snapshot: &pb.ChangeSnapshot{Data: s.Snapshot},
			}
		}
		res := &types2.Response{
			Snapshots: sn,
		}

		originImport := objectorigin.Import(model.Import_External)
		details, _ := i.createObjects(ctx, res, progress, req, allErrors, originImport)
		if !allErrors.IsEmpty() {
			return 0, allErrors.GetResultError(req.Type)
		}
		return int64(len(details)), nil
	}
	return 0, types2.ErrNoObjectsToImport
}

func (i *Import) finishImportProcess(returnedErr error, progress process.Progress) {
	progress.Finish(returnedErr)
}

func shouldReturnError(e error, res *types2.Response, req *pb.RpcObjectImportRequest) bool {
	return (e != nil && req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS) ||
		errors.Is(e, types2.ErrFailedToReceiveListOfObjects) || errors.Is(e, types2.ErrLimitExceeded) ||
		(errors.Is(e, types2.ErrNoObjectsToImport) && (res == nil || len(res.Snapshots) == 0)) || // return error only if we don't have object to import
		errors.Is(e, types2.ErrCancel)
}

func (i *Import) setupProgressBar(req *pb.RpcObjectImportRequest) process.Progress {
	progressBarType := pb.ModelProcess_Import
	if req.IsMigration {
		progressBarType = pb.ModelProcess_Migration
	}
	var progress process.Progress
	if req.GetNoProgress() {
		progress = process.NewNoOp()
	} else {
		progress = process.NewProgress(progressBarType)
	}
	return progress
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

func (i *Import) ImportWeb(ctx context.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error) {
	progress := process.NewProgress(pb.ModelProcess_Import)
	defer progress.Finish(nil)
	allErrors := types2.NewError(0)

	progress.SetProgressMessage("Parse url")
	w := i.converters[web.Name]
	res, err := w.GetSnapshots(ctx, req, progress)

	if err != nil {
		return "", nil, err.Error()
	}
	if res.Snapshots == nil || len(res.Snapshots) == 0 {
		return "", nil, fmt.Errorf("snpashots are empty")
	}

	progress.SetProgressMessage("Create objects")
	details, _ := i.createObjects(ctx, res, progress, req, allErrors, objectorigin.None())
	if !allErrors.IsEmpty() {
		return "", nil, fmt.Errorf("couldn't create objects")
	}
	return res.Snapshots[0].Id, details[res.Snapshots[0].Id], nil
}

func (i *Import) createObjects(ctx context.Context,
	res *types2.Response,
	progress process.Progress,
	req *pb.RpcObjectImportRequest,
	allErrors *types2.ConvertError,
	origin objectorigin.ObjectOrigin,
) (map[string]*types.Struct, string) {
	oldIDToNew, createPayloads, err := i.getIDForAllObjects(ctx, res, allErrors, req, origin)
	if err != nil {
		return nil, ""
	}
	filesIDs := i.getFilesIDs(res)
	numWorkers := workerPoolSize
	if len(res.Snapshots) < workerPoolSize {
		numWorkers = 1
	}
	do := creator.NewDataObject(ctx, oldIDToNew, createPayloads, filesIDs, origin, req.SpaceId)
	pool := workerpool.NewPool(numWorkers)
	progress.SetProgressMessage("Create objects")
	go i.addWork(res, pool)
	go pool.Start(do)
	details := i.readResultFromPool(pool, req.Mode, allErrors, progress)
	return details, oldIDToNew[res.RootCollectionID]
}

func (i *Import) getFilesIDs(res *types2.Response) []string {
	fileIDs := make([]string, 0)
	for _, snapshot := range res.Snapshots {
		fileIDs = append(fileIDs, lo.Map(snapshot.Snapshot.GetFileKeys(), func(item *pb.ChangeFileKeys, index int) string {
			return item.Hash
		})...)
	}
	return fileIDs
}

func (i *Import) getIDForAllObjects(ctx context.Context,
	res *types2.Response,
	allErrors *types2.ConvertError,
	req *pb.RpcObjectImportRequest,
	origin objectorigin.ObjectOrigin,
) (map[string]string, map[string]treestorage.TreeStorageCreatePayload, error) {
	relationOptions := make([]*types2.Snapshot, 0)
	oldIDToNew := make(map[string]string, len(res.Snapshots))
	createPayloads := make(map[string]treestorage.TreeStorageCreatePayload, len(res.Snapshots))
	for _, snapshot := range res.Snapshots {
		// we will get id of relation options after we figure out according relations keys
		if lo.Contains(snapshot.Snapshot.GetData().GetObjectTypes(), bundle.TypeKeyRelationOption.String()) {
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

func (i *Import) replaceRelationKeyWithNew(option *types2.Snapshot, oldIDToNew map[string]string) {
	if option.Snapshot.Data.Details == nil || len(option.Snapshot.Data.Details.Fields) == 0 {
		return
	}
	key := pbtypes.GetString(option.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String())
	if newRelationID, ok := oldIDToNew[key]; ok {
		key = strings.TrimPrefix(newRelationID, addr.RelationKeyToIdPrefix)
	}
	option.Snapshot.Data.Details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
}

func (i *Import) getObjectID(
	ctx context.Context,
	spaceID string,
	snapshot *types2.Snapshot,
	createPayloads map[string]treestorage.TreeStorageCreatePayload,
	oldIDToNew map[string]string,
	updateExisting bool,
	origin objectorigin.ObjectOrigin,
) error {

	// Preload file keys
	for _, fileKeys := range snapshot.Snapshot.GetFileKeys() {
		err := i.fileStore.AddFileKeys(domain.FileEncryptionKeys{
			FileId:         domain.FileId(fileKeys.Hash),
			EncryptionKeys: fileKeys.Keys,
		})
		if err != nil {
			return fmt.Errorf("add file keys: %w", err)
		}
	}
	if fileInfo := snapshot.Snapshot.GetData().GetFileInfo(); fileInfo != nil {
		keys := make(map[string]string, len(fileInfo.EncryptionKeys))
		for _, key := range fileInfo.EncryptionKeys {
			keys[key.Path] = key.Key
		}
		err := i.fileStore.AddFileKeys(domain.FileEncryptionKeys{
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
	if payload.RootRawChange != nil {
		createPayloads[id] = payload
	}
	return i.extractInternalKey(snapshot, oldIDToNew)
}

func (i *Import) extractInternalKey(snapshot *types2.Snapshot, oldIDToNew map[string]string) error {
	newUniqueKey := i.idProvider.GetInternalKey(snapshot.SbType)
	if newUniqueKey != "" {
		oldUniqueKey := pbtypes.GetString(snapshot.Snapshot.Data.Details, bundle.RelationKeyUniqueKey.String())
		if oldUniqueKey == "" {
			oldUniqueKey = snapshot.Snapshot.Data.Key
		}
		if oldUniqueKey != "" {
			oldIDToNew[oldUniqueKey] = newUniqueKey
		}
	}
	return nil
}

func (i *Import) addWork(res *types2.Response, pool *workerpool.WorkerPool) {
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
	allErrors *types2.ConvertError,
	progress process.Progress,
) map[string]*types.Struct {
	details := make(map[string]*types.Struct, 0)
	for r := range pool.Results() {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(fmt.Errorf("%w: %s", types2.ErrCancel, err.Error()))
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

func convertType(cType string) pb.RpcObjectImportListImportResponseType {
	return pb.RpcObjectImportListImportResponseType(pb.RpcObjectImportListImportResponseType_value[cType])
}
