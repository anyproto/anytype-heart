package importer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/csv"
	"github.com/anyproto/anytype-heart/core/block/import/html"
	"github.com/anyproto/anytype-heart/core/block/import/markdown"
	"github.com/anyproto/anytype-heart/core/block/import/notion"
	pbc "github.com/anyproto/anytype-heart/core/block/import/pb"
	"github.com/anyproto/anytype-heart/core/block/import/syncer"
	"github.com/anyproto/anytype-heart/core/block/import/txt"
	"github.com/anyproto/anytype-heart/core/block/import/web"
	"github.com/anyproto/anytype-heart/core/block/import/workerpool"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("import")

const CName = "importer"

const workerPoolSize = 10

type Import struct {
	converters      map[string]converter.Converter
	s               *block.Service
	oc              Creator
	objectIDGetter  IDGetter
	tempDirProvider core.TempDirProvider
	sbtProvider     typeprovider.SmartBlockTypeProvider
}

func New() Importer {
	return &Import{
		converters: make(map[string]converter.Converter, 0),
	}
}

func (i *Import) Init(a *app.App) (err error) {
	i.s = a.MustComponent(block.CName).(*block.Service)
	coreService := a.MustComponent(core.CName).(core.Service)
	col := app.MustComponent[*collection.Service](a)
	i.tempDirProvider = app.MustComponent[core.TempDirProvider](a)
	converters := []converter.Converter{
		markdown.New(i.tempDirProvider, col),
		notion.New(col),
		pbc.New(col, i.sbtProvider, coreService),
		web.NewConverter(),
		html.New(col),
		txt.New(col),
		csv.New(col),
	}
	for _, c := range converters {
		i.converters[c.Name()] = c
	}

	factory := syncer.New(syncer.NewFileSyncer(i.s), syncer.NewBookmarkSyncer(i.s), syncer.NewIconSyncer(i.s))
	objCreator := a.MustComponent(objectcreator.CName).(objectCreator)
	store := app.MustComponent[objectstore.ObjectStore](a)
	i.objectIDGetter = NewObjectIDGetter(store, coreService, i.s)
	fileStore := app.MustComponent[filestore.FileStore](a)
	relationSyncer := syncer.NewFileRelationSyncer(i.s, fileStore)
	i.oc = NewCreator(i.s, objCreator, coreService, factory, store, relationSyncer, fileStore)
	i.sbtProvider = app.MustComponent[typeprovider.SmartBlockTypeProvider](a)
	return nil
}

// Import get snapshots from converter or external api and create smartblocks from them
func (i *Import) Import(ctx *session.Context, req *pb.RpcObjectImportRequest) error {
	progress := i.setupProgressBar(req)
	var returnedErr error
	defer func() {
		i.finishImportProcess(returnedErr, progress)
	}()
	if i.s != nil && !req.GetNoProgress() {
		i.s.ProcessAdd(progress)
	}
	if c, ok := i.converters[req.Type.String()]; ok {
		returnedErr = i.importFromBuiltinConverter(ctx, req, c, progress)
		return returnedErr
	}
	if req.Type == pb.RpcObjectImportRequest_External {
		returnedErr = i.importFromExternalSource(ctx, req, progress)
		return returnedErr
	}
	returnedErr = fmt.Errorf("unknown import type %s", req.Type)
	return returnedErr
}

func (i *Import) importFromBuiltinConverter(ctx *session.Context,
	req *pb.RpcObjectImportRequest,
	c converter.Converter,
	progress process.Progress) error {
	allErrors := converter.NewError()
	res, err := c.GetSnapshots(req, progress)
	if !err.IsEmpty() {
		resultErr := err.GetResultError(req.Type)
		if shouldReturnError(resultErr, res, req) {
			return resultErr
		}
		allErrors.Merge(err)
	}
	if res == nil {
		return fmt.Errorf("source path doesn't contain %s resources to import", req.Type)
	}

	if len(res.Snapshots) == 0 {
		return fmt.Errorf("source path doesn't contain %s resources to import", req.Type)
	}

	i.createObjects(ctx, res, progress, req, allErrors)
	return allErrors.GetResultError(req.Type)
}

func (i *Import) importFromExternalSource(ctx *session.Context, req *pb.RpcObjectImportRequest, progress process.Progress) error {
	allErrors := converter.NewError()
	if req.Snapshots != nil {
		sn := make([]*converter.Snapshot, len(req.Snapshots))
		for i, s := range req.Snapshots {
			sn[i] = &converter.Snapshot{
				Id:       s.GetId(),
				Snapshot: &pb.ChangeSnapshot{Data: s.Snapshot},
			}
		}
		res := &converter.Response{
			Snapshots: sn,
		}
		i.createObjects(ctx, res, progress, req, allErrors)
		if !allErrors.IsEmpty() {
			return allErrors.GetResultError(req.Type)
		}
		return nil
	}
	return converter.ErrNoObjectsToImport
}

func (i *Import) finishImportProcess(returnedErr error, progress process.Progress) {
	progress.Finish(returnedErr)
}

func shouldReturnError(e error, res *converter.Response, req *pb.RpcObjectImportRequest) bool {
	return (e != nil && req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS) ||
		errors.Is(e, converter.ErrFailedToReceiveListOfObjects) || errors.Is(e, converter.ErrLimitExceeded) ||
		(errors.Is(e, converter.ErrNoObjectsToImport) && (res == nil || len(res.Snapshots) == 0)) || // return error only if we don't have object to import
		errors.Is(e, converter.ErrCancel)
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
func (i *Import) ListImports(_ *session.Context,
	_ *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error) {
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

func (i *Import) ImportWeb(ctx *session.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error) {
	progress := process.NewProgress(pb.ModelProcess_Import)
	defer progress.Finish(nil)
	allErrors := converter.NewError()

	progress.SetProgressMessage("Parse url")
	w := i.converters[web.Name]
	res, err := w.GetSnapshots(req, progress)

	if err != nil {
		return "", nil, err.Error()
	}
	if res.Snapshots == nil || len(res.Snapshots) == 0 {
		return "", nil, fmt.Errorf("snpashots are empty")
	}

	progress.SetProgressMessage("Create objects")
	details := i.createObjects(ctx, res, progress, req, allErrors)
	if !allErrors.IsEmpty() {
		return "", nil, fmt.Errorf("couldn't create objects")
	}
	return res.Snapshots[0].Id, details[res.Snapshots[0].Id], nil
}

func (i *Import) createObjects(ctx *session.Context, res *converter.Response, progress process.Progress, req *pb.RpcObjectImportRequest, allErrors *converter.ConvertError) map[string]*types.Struct {
	oldIDToNew, createPayloads, err := i.getIDForAllObjects(ctx, res, allErrors, req)
	if err != nil {
		return nil
	}
	filesIDs := i.getFilesIDs(res)
	numWorkers := workerPoolSize
	if len(res.Snapshots) < workerPoolSize {
		numWorkers = 1
	}
	do := NewDataObject(oldIDToNew, createPayloads, filesIDs, ctx)
	pool := workerpool.NewPool(numWorkers)
	progress.SetProgressMessage("Create objects")
	go i.addWork(res, pool)
	go pool.Start(do)
	details := i.readResultFromPool(pool, req.Mode, allErrors, progress)
	return details
}

func (i *Import) getFilesIDs(res *converter.Response) []string {
	fileIDs := make([]string, 0)
	for _, snapshot := range res.Snapshots {
		fileIDs = append(fileIDs, lo.Map(snapshot.Snapshot.GetFileKeys(), func(item *pb.ChangeFileKeys, index int) string {
			return item.Hash
		})...)
	}
	return fileIDs
}

func (i *Import) getIDForAllObjects(ctx *session.Context, res *converter.Response, allErrors *converter.ConvertError, req *pb.RpcObjectImportRequest) (map[string]string, map[string]treestorage.TreeStorageCreatePayload, error) {
	relationOptions := make([]*converter.Snapshot, 0)
	oldIDToNew := make(map[string]string, len(res.Snapshots))
	createPayloads := make(map[string]treestorage.TreeStorageCreatePayload, len(res.Snapshots))
	for _, snapshot := range res.Snapshots {
		// we will get id of relation options after we figure out according relations keys
		if lo.Contains(snapshot.Snapshot.GetData().GetObjectTypes(), bundle.TypeKeyRelationOption.URL()) {
			relationOptions = append(relationOptions, snapshot)
			continue
		}
		err := i.getObjectID(ctx, snapshot, createPayloads, oldIDToNew, req.UpdateExistingObjects)
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
		err := i.getObjectID(ctx, option, createPayloads, oldIDToNew, req.UpdateExistingObjects)
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

func (i *Import) replaceRelationKeyWithNew(option *converter.Snapshot, oldIDToNew map[string]string) {
	if option.Snapshot.Data.Details == nil || len(option.Snapshot.Data.Details.Fields) == 0 {
		return
	}
	key := pbtypes.GetString(option.Snapshot.Data.Details, bundle.RelationKeyRelationKey.String())
	relationID := addr.RelationKeyToIdPrefix + key
	if newRelationID, ok := oldIDToNew[relationID]; ok {
		key = strings.TrimPrefix(newRelationID, addr.RelationKeyToIdPrefix)
	}
	option.Snapshot.Data.Details.Fields[bundle.RelationKeyRelationKey.String()] = pbtypes.String(key)
}

func (i *Import) getObjectID(ctx *session.Context,
	snapshot *converter.Snapshot,
	createPayloads map[string]treestorage.TreeStorageCreatePayload,
	oldIDToNew map[string]string,
	updateExisting bool) error {
	var (
		err         error
		id          string
		payload     treestorage.TreeStorageCreatePayload
		createdTime time.Time
	)
	createdTimeTS := pbtypes.GetInt64(snapshot.Snapshot.GetData().GetDetails(), bundle.RelationKeyCreatedDate.String())
	if createdTimeTS > 0 {
		createdTime = time.Unix(createdTimeTS, 0)
	} else {
		createdTime = time.Now()
	}
	if id, payload, err = i.objectIDGetter.Get(ctx, snapshot, snapshot.SbType, createdTime, updateExisting, oldIDToNew); err == nil {
		oldIDToNew[snapshot.Id] = id
		if snapshot.SbType == sb.SmartBlockTypeSubObject && id == "" {
			oldIDToNew[snapshot.Id] = snapshot.Id
		}
		if payload.RootRawChange != nil {
			createPayloads[id] = payload
		}
		return nil
	}
	return err
}

func (i *Import) addWork(res *converter.Response, pool *workerpool.WorkerPool) {
	for _, snapshot := range res.Snapshots {
		t := NewTask(snapshot, i.oc)
		stop := pool.AddWork(t)
		if stop {
			break
		}
	}
	pool.CloseTask()
}

func (i *Import) readResultFromPool(pool *workerpool.WorkerPool, mode pb.RpcObjectImportRequestMode, allErrors *converter.ConvertError, progress process.Progress) map[string]*types.Struct {
	details := make(map[string]*types.Struct, 0)
	for r := range pool.Results() {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(errors.Wrap(converter.ErrCancel, err.Error()))
			pool.Stop()
			return nil
		}
		res := r.(*Result)
		if res.err != nil {
			allErrors.Add(res.err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				pool.Stop()
				return nil
			}
		}
		details[res.newID] = res.details
	}
	return details
}

func convertType(cType string) pb.RpcObjectImportListImportResponseType {
	return pb.RpcObjectImportListImportResponseType(pb.RpcObjectImportListImportResponseType_value[cType])
}
