package importer

import (
	"context"
	"fmt"
	"github.com/anytypeio/any-sync/app"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/csv"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/html"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/markdown"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/notion"
	pbc "github.com/anytypeio/go-anytype-middleware/core/block/import/pb"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/syncer"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/txt"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/web"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/workerpool"
	"github.com/anytypeio/go-anytype-middleware/core/block/object"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	sb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
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

	factory := syncer.New(syncer.NewFileSyncer(i.s), syncer.NewIconSyncer(i.s))
	fs := a.MustComponent(filestore.CName).(filestore.FileStore)
	objCreator := a.MustComponent(object.CName).(objectCreator)
	store := app.MustComponent[objectstore.ObjectStore](a)
	relationCreator := NewRelationCreator(i.s, objCreator, fs, coreService, store)
	i.objectIDGetter = NewObjectIDGetter(store, coreService, i.s)
	fileStore := app.MustComponent[filestore.FileStore](a)
	i.oc = NewCreator(i.s, objCreator, coreService, factory, relationCreator, store, fileStore)
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
			allErrors.Merge(err)
			if req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
				return allErrors.Error()
			}
		}

		if res == nil {
			return fmt.Errorf("no files to import")
		}

		if len(res.Snapshots) == 0 {
			return fmt.Errorf("no files to import")
		}

		i.createObjects(ctx, res, progress, req, allErrors)
		return allErrors.Error()
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
			return allErrors.Error()
		}
		return fmt.Errorf("snapshots are empty")
	}
	return fmt.Errorf("unknown import type %s", req.Type)
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
func (i *Import) ValidateNotionToken(ctx context.Context,
	req *pb.RpcObjectImportNotionValidateTokenRequest) pb.RpcObjectImportNotionValidateTokenResponseErrorCode {
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
	getFileName := func(object *converter.Snapshot) string {
		if object.FileName != "" {
			return object.FileName
		}
		if object.Id != "" {
			return object.Id
		}
		return ""
	}

	oldIDToNew := make(map[string]string, len(res.Snapshots))
	existedObject := make(map[string]struct{}, 0)
	for _, snapshot := range res.Snapshots {
		var (
			err   error
			id    string
			exist bool
		)

		if id, exist, err = i.objectIDGetter.Get(ctx, snapshot, snapshot.SbType, req.UpdateExistingObjects); err == nil {
			oldIDToNew[snapshot.Id] = id
			if snapshot.SbType == sb.SmartBlockTypeSubObject && id == "" {
				oldIDToNew[snapshot.Id] = snapshot.Id
			}
			if exist {
				existedObject[snapshot.Id] = struct{}{}
			}
			continue
		}
		if err != nil {
			allErrors[getFileName(snapshot)] = err
			if req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
				return nil
			}
			log.With(zap.String("object name", getFileName(snapshot))).Error(err)
		}
	}
	numWorkers := workerPoolSize
	if len(res.Snapshots) < workerPoolSize {
		numWorkers = 1
	}
	do := NewDataObject(oldIDToNew, ctx)
	pool := workerpool.NewPool(numWorkers)
	progress.SetProgressMessage("Create objects")
	go i.addWork(res, existedObject, pool)
	go pool.Start(do)
	details := i.readResultFromPool(pool, req.Mode, allErrors, progress)
	return details
}

func (i *Import) addWork(res *converter.Response, existedObject map[string]struct{}, pool *workerpool.WorkerPool) {
	for _, snapshot := range res.Snapshots {
		var relations []*converter.Relation
		if res.Relations != nil {
			relations = res.Relations[snapshot.Id]
		}
		_, ok := existedObject[snapshot.Id]
		t := NewTask(snapshot, relations, ok, i.oc)
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
			allErrors["cancel error"] = err
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
