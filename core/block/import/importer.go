package importer

import (
	"context"
	"fmt"
	"github.com/samber/lo"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"
	"github.com/pkg/errors"
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

func New(
	tempDirProvider core.TempDirProvider,
	sbtProvider typeprovider.SmartBlockTypeProvider,
) Importer {
	return &Import{
		tempDirProvider: tempDirProvider,
		sbtProvider:     sbtProvider,
		converters:      make(map[string]converter.Converter, 0),
	}
}

func (i *Import) Init(a *app.App) (err error) {
	i.s = a.MustComponent(block.CName).(*block.Service)
	coreService := a.MustComponent(core.CName).(core.Service)
	col := app.MustComponent[*collection.Service](a)
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
	i.oc = NewCreator(i.s, objCreator, coreService, factory, store, relationSyncer)
	return nil
}

// Import get snapshots from converter or external api and create smartblocks from them
func (i *Import) Import(ctx *session.Context, req *pb.RpcObjectImportRequest) error {
	progress := i.setupProgressBar(req)
	defer progress.Finish()
	if i.s != nil && !req.GetNoProgress() {
		i.s.ProcessAdd(progress)
	}
	allErrors := converter.NewError()
	if c, ok := i.converters[req.Type.String()]; ok {
		res, err := c.GetSnapshots(req, progress)
		if len(err) != 0 {
			e := getResultError(err)
			if shouldReturnError(e, req) {
				return e
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
		return getResultError(allErrors)
	}
	if req.Type == pb.RpcObjectImportRequest_External {
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
				return getResultError(allErrors)
			}
			return nil
		}
		return converter.ErrNoObjectsToImport
	}
	return fmt.Errorf("unknown import type %s", req.Type)
}

func shouldReturnError(e error, req *pb.RpcObjectImportRequest) bool {
	return (e != nil && req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS) ||
		e == converter.ErrNoObjectsToImport ||
		e == converter.ErrCancel
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
	defer progress.Finish()
	allErrors := make(map[string]error, 0)

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
	if len(allErrors) != 0 {
		return "", nil, fmt.Errorf("couldn't create objects")
	}

	return res.Snapshots[0].Id, details[res.Snapshots[0].Id], nil
}

func (i *Import) createObjects(ctx *session.Context,
	res *converter.Response,
	progress process.Progress,
	req *pb.RpcObjectImportRequest,
	allErrors map[string]error) map[string]*types.Struct {

	oldIDToNew, createPayloads, err := i.getIDForAllObjects(ctx, res, allErrors, req)
	if err != nil {
		return nil
	}
	numWorkers := workerPoolSize
	if len(res.Snapshots) < workerPoolSize {
		numWorkers = 1
	}
	do := NewDataObject(oldIDToNew, createPayloads, ctx)
	pool := workerpool.NewPool(numWorkers)
	progress.SetProgressMessage("Create objects")
	go i.addWork(res, pool)
	go pool.Start(do)
	details := i.readResultFromPool(pool, req.Mode, allErrors, progress)
	return details
}

func (i *Import) getIDForAllObjects(ctx *session.Context, res *converter.Response, allErrors map[string]error, req *pb.RpcObjectImportRequest) (
	map[string]string, map[string]treestorage.TreeStorageCreatePayload, error) {
	getFileName := func(object *converter.Snapshot) string {
		if object.FileName != "" {
			return object.FileName
		}
		if object.Id != "" {
			return object.Id
		}
		return ""
	}
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
			allErrors[getFileName(snapshot)] = err
			if req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
				return nil, nil, err
			}
			log.With(zap.String("object name", getFileName(snapshot))).Error(err)
		}
	}
	for _, option := range relationOptions {
		err := i.getObjectID(ctx, option, createPayloads, oldIDToNew, req.UpdateExistingObjects)
		if err != nil {
			allErrors[getFileName(option)] = err
			if req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
				return nil, nil, err
			}
			log.With(zap.String("object name", getFileName(option))).Error(err)
		}
	}
	return oldIDToNew, createPayloads, nil
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
	if id, payload, err = i.objectIDGetter.Get(ctx, snapshot, snapshot.SbType, createdTime, updateExisting); err == nil {
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

func (i *Import) readResultFromPool(pool *workerpool.WorkerPool,
	mode pb.RpcObjectImportRequestMode,
	allErrors map[string]error,
	progress process.Progress) map[string]*types.Struct {
	details := make(map[string]*types.Struct, 0)
	for r := range pool.Results() {
		if err := progress.TryStep(1); err != nil {
			wrappedError := errors.Wrap(converter.ErrCancel, err.Error())
			allErrors["cancel error"] = wrappedError
			pool.Stop()
			return nil
		}
		res := r.(*Result)
		if res.err != nil {
			allErrors[res.newID] = res.err
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

func getResultError(err converter.ConvertError) error {
	if err.IsEmpty() {
		return nil
	}
	var countNoObjectsToImport int
	for _, e := range err {
		switch {
		case errors.Is(e, converter.ErrCancel):
			return converter.ErrCancel
		case errors.Is(e, converter.ErrNoObjectsToImport):
			countNoObjectsToImport++
		}
	}
	// we return ErrNoObjectsToImport only if all paths has such error, otherwise we assume that import finished with internal code error
	if countNoObjectsToImport == len(err) {
		return converter.ErrNoObjectsToImport
	}
	return err.Error()
}
