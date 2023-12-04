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
	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/metrics"
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
	converters      map[string]common.Converter
	s               *block.Service
	oc              creator.Service
	idProvider      objectid.IDProvider
	tempDirProvider core.TempDirProvider
	fileSync        filesync.FileSync
	sync.Mutex
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
	col := app.MustComponent[*collection.Service](a)
	i.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	converters := []common.Converter{
		markdown.New(i.tempDirProvider, col),
		notion.New(col),
		pbc.New(col, accountService),
		web.NewConverter(),
		html.New(col, i.tempDirProvider),
		txt.New(col),
		csv.New(col),
	}
	for _, c := range converters {
		i.converters[c.Name()] = c
	}
	resolver := a.MustComponent(idresolver.CName).(idresolver.Resolver)
	factory := syncer.New(syncer.NewFileSyncer(i.s), syncer.NewBookmarkSyncer(i.s), syncer.NewIconSyncer(i.s, resolver))
	store := app.MustComponent[objectstore.ObjectStore](a)
	i.idProvider = objectid.NewIDProvider(store, spaceService)
	fileStore := app.MustComponent[filestore.FileStore](a)
	relationSyncer := syncer.NewFileRelationSyncer(i.s, fileStore)
	objectCreator := app.MustComponent[objectcreator.Service](a)
	i.oc = creator.New(i.s, factory, store, relationSyncer, fileStore, spaceService, objectCreator)
	i.fileSync = app.MustComponent[filesync.FileSync](a)
	return nil
}

// Import get snapshots from converter or external api and create smartblocks from them
func (i *Import) Import(ctx context.Context, req *pb.RpcObjectImportRequest, origin model.ObjectOrigin, progress process.Progress) (string, string, error) {
	if req.SpaceId == "" {
		return "", "", fmt.Errorf("spaceId is empty")
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
	var rootCollectionID string
	if c, ok := i.converters[req.Type.String()]; ok {
		rootCollectionID, returnedErr = i.importFromBuiltinConverter(ctx, req, c, progress, origin)
		return rootCollectionID, "", returnedErr
	}
	if req.Type == model.ImportType_External {
		returnedErr = i.importFromExternalSource(ctx, req, progress)
		return rootCollectionID, "", returnedErr
	}
	returnedErr = fmt.Errorf("unknown import type %s", req.Type)
	return rootCollectionID, progress.Id(), returnedErr
}

func (i *Import) sendFileEvents(returnedErr error) {
	if returnedErr == nil {
		i.fileSync.SendImportEvents()
	}
	i.fileSync.ClearImportEvents()
}

func (i *Import) importFromBuiltinConverter(ctx context.Context,
	req *pb.RpcObjectImportRequest,
	c common.Converter,
	progress process.Progress,
	origin model.ObjectOrigin,
) (string, error) {
	allErrors := common.NewError(req.Mode)
	res, err := c.GetSnapshots(ctx, req, progress)
	if !err.IsEmpty() {
		resultErr := err.GetResultError(req.Type)
		if shouldReturnError(resultErr, res, req) {
			return "", resultErr
		}
		allErrors.Merge(err)
	}
	if res == nil {
		return "", fmt.Errorf("source path doesn't contain %s resources to import", req.Type)
	}

	if len(res.Snapshots) == 0 {
		return "", fmt.Errorf("source path doesn't contain %s resources to import", req.Type)
	}

	_, rootCollectionID := i.createObjects(ctx, res, progress, req, allErrors, origin)
	resultErr := allErrors.GetResultError(req.Type)
	if resultErr != nil {
		rootCollectionID = ""
	}
	return rootCollectionID, resultErr
}

func (i *Import) importFromExternalSource(ctx context.Context,
	req *pb.RpcObjectImportRequest,
	progress process.Progress,
) error {
	allErrors := common.NewError(req.Mode)
	if req.Snapshots != nil {
		sn := make([]*common.Snapshot, len(req.Snapshots))
		for i, s := range req.Snapshots {
			sn[i] = &common.Snapshot{
				Id:       s.GetId(),
				Snapshot: &pb.ChangeSnapshot{Data: s.Snapshot},
			}
		}
		res := &common.Response{
			Snapshots: sn,
		}
		i.createObjects(ctx, res, progress, req, allErrors, model.ObjectOrigin_import)
		if !allErrors.IsEmpty() {
			return allErrors.GetResultError(req.Type)
		}
		return nil
	}
	return common.ErrNoObjectsToImport
}

func (i *Import) finishImportProcess(returnedErr error, progress process.Progress) {
	progress.Finish(returnedErr)
}

func shouldReturnError(e error, res *common.Response, req *pb.RpcObjectImportRequest) bool {
	return (e != nil && req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS) ||
		errors.Is(e, common.ErrFailedToReceiveListOfObjects) || errors.Is(e, common.ErrLimitExceeded) ||
		(errors.Is(e, common.ErrNoObjectsToImport) && (res == nil || len(res.Snapshots) == 0)) || // return error only if we don't have object to import
		errors.Is(e, common.ErrCancel)
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
	allErrors := common.NewError(0)

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
	details, _ := i.createObjects(ctx, res, progress, req, allErrors, model.ObjectOrigin_import)
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
	origin model.ObjectOrigin,
) (map[string]*types.Struct, string) {
	oldIDToNew, createPayloads, err := i.getIDForAllObjects(ctx, res, allErrors, req)
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
	go i.addWork(req.SpaceId, res, pool)
	go pool.Start(do)
	details := i.readResultFromPool(pool, req.Mode, allErrors, progress)
	return details, oldIDToNew[res.RootCollectionID]
}

func (i *Import) getFilesIDs(res *common.Response) []string {
	fileIDs := make([]string, 0)
	for _, snapshot := range res.Snapshots {
		fileIDs = append(fileIDs, lo.Map(snapshot.Snapshot.GetFileKeys(), func(item *pb.ChangeFileKeys, index int) string {
			return item.Hash
		})...)
	}
	return fileIDs
}

func (i *Import) getIDForAllObjects(ctx context.Context,
	res *common.Response,
	allErrors *common.ConvertError,
	req *pb.RpcObjectImportRequest,
) (map[string]string, map[string]treestorage.TreeStorageCreatePayload, error) {
	relationOptions := make([]*common.Snapshot, 0)
	oldIDToNew := make(map[string]string, len(res.Snapshots))
	createPayloads := make(map[string]treestorage.TreeStorageCreatePayload, len(res.Snapshots))
	for _, snapshot := range res.Snapshots {
		// we will get id of relation options after we figure out according relations keys
		if lo.Contains(snapshot.Snapshot.GetData().GetObjectTypes(), bundle.TypeKeyRelationOption.String()) {
			relationOptions = append(relationOptions, snapshot)
			continue
		}
		err := i.getObjectID(ctx, req.SpaceId, snapshot, createPayloads, oldIDToNew, req.UpdateExistingObjects)
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
		err := i.getObjectID(ctx, req.SpaceId, option, createPayloads, oldIDToNew, req.UpdateExistingObjects)
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
	snapshot *common.Snapshot,
	createPayloads map[string]treestorage.TreeStorageCreatePayload,
	oldIDToNew map[string]string,
	updateExisting bool,
) error {
	var (
		id      string
		payload treestorage.TreeStorageCreatePayload
	)

	id, payload, err := i.idProvider.GetIDAndPayload(ctx, spaceID, snapshot, time.Now(), updateExisting)
	if err != nil {
		return err
	}
	oldIDToNew[snapshot.Id] = id
	if payload.RootRawChange != nil {
		createPayloads[id] = payload
	}
	return nil
}

func (i *Import) addWork(spaceID string, res *common.Response, pool *workerpool.WorkerPool) {
	for _, snapshot := range res.Snapshots {
		t := creator.NewTask(spaceID, snapshot, i.oc)
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
) map[string]*types.Struct {
	details := make(map[string]*types.Struct, 0)
	for r := range pool.Results() {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(fmt.Errorf("%w: %w", common.ErrCancel, err))
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
func (i *Import) recordEvent(event metrics.EventRepresentable) {
	metrics.SharedClient.RecordEvent(event)
}

func convertType(cType string) pb.RpcObjectImportListImportResponseType {
	return pb.RpcObjectImportListImportResponseType(pb.RpcObjectImportListImportResponseType_value[cType])
}
