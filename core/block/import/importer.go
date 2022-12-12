package importer

import (
	"fmt"

	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anytypeio/go-anytype-middleware/app"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/syncer"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/web"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
)

var log = logging.Logger("import")

const CName = "importer"

type Import struct {
	converters map[string]converter.Converter
	s          block.Service
	oc         Creator
}

func New() Importer {
	return &Import{
		converters: make(map[string]converter.Converter, 0),
	}
}

func (i *Import) Init(a *app.App) (err error) {
	i.s = a.MustComponent(block.CName).(block.Service)
	core := a.MustComponent(core.CName).(core.Service)
	for _, f := range converter.GetConverters() {
		converter := f(core)
		i.converters[converter.Name()] = converter
	}
	factory := syncer.New(syncer.NewFileSyncer(i.s), syncer.NewBookmarkSyncer(i.s), syncer.NewIconSyncer(i.s))
	ou := NewObjectUpdater(i.s, core, factory)
	relationCreator := NewRelationCreator(i.s)
	i.oc = NewCreator(i.s, core, ou, factory, relationCreator)
	return nil
}

// Import get snapshots from converter or external api and create smartblocks from them
func (i *Import) Import(ctx *session.Context, req *pb.RpcObjectImportRequest) error {
	progress := process.NewProgress(pb.ModelProcess_Import)
	defer progress.Finish()
	if i.s != nil {
		i.s.ProcessAdd(progress)
	}
	allErrors := converter.NewError()
	if c, ok := i.converters[req.Type.String()]; ok {
		res, err := c.GetSnapshots(req, progress)
		if res == nil {
			return fmt.Errorf("no files to import")
		}

		if len(err) != 0 {
			allErrors.Merge(err)
			if req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
				return allErrors.Error()
			}
		}
		if len(res.Snapshots) == 0 {
			return fmt.Errorf("no files to import")
		}

		progress.SetProgressMessage("Create objects")
		i.createObjects(ctx, res, progress, req, allErrors)
		return allErrors.Error()
	}
	if req.Type == pb.RpcObjectImportRequest_External {
		if req.Snapshots != nil {
			sn := make([]*converter.Snapshot, len(req.Snapshots))
			for i, s := range req.Snapshots {
				sn[i] = &converter.Snapshot{
					Id:       s.GetId(),
					Snapshot: s.Snapshot,
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

func (s *Import) Name() string {
	return CName
}

// ListImports return all registered import types
func (i *Import) ListImports(ctx *session.Context, req *pb.RpcObjectImportListRequest) ([]*pb.RpcObjectImportListImportResponse, error) {
	res := make([]*pb.RpcObjectImportListImportResponse, len(i.converters))
	var idx int
	for _, c := range i.converters {
		res[idx] = &pb.RpcObjectImportListImportResponse{Type: convertType(c.Name())}
		idx++
	}
	return res, nil
}

func (i *Import) ImportWeb(ctx *session.Context, req *pb.RpcObjectImportRequest) (string, *types.Struct, error) {
	progress := process.NewProgress(pb.ModelProcess_Import)
	defer progress.Finish()
	allErrors := make(map[string]error, 0)

	progress.SetProgressMessage("Parse url")
	w := web.NewConverter()
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

func (i *Import) createObjects(ctx *session.Context, res *converter.Response, progress *process.Progress, req *pb.RpcObjectImportRequest, allErrors map[string]error) map[string]*types.Struct {
	var (
		sbType smartblock.SmartBlockType
		err    error
	)
	getFileName := func(object *converter.Snapshot) string {
		if object.FileName != "" {
			return object.FileName
		}
		if object.Id != "" {
			return object.Id
		}
		return ""
	}

	details := make(map[string]*types.Struct, 0)

	for _, snapshot := range res.Snapshots {
		switch {
		case snapshot.Id != "":
			sbType, err = smartblock.SmartBlockTypeFromID(snapshot.Id)
			if err != nil {
				allErrors[getFileName(snapshot)] = err
				if req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
					return nil
				}
				log.With(zap.String("object name", snapshot.Id)).Error(err)
			}
		default:
			sbType = smartblock.SmartBlockTypePage
		}

		if err := progress.TryStep(1); err != nil {
			allErrors[getFileName(snapshot)] = err
			return nil
		}
		var relations []*converter.Relation
		if res.Relations != nil {
			relations = res.Relations[snapshot.Id]
		}
		detail, err := i.oc.Create(ctx, snapshot.Snapshot, relations, snapshot.Id, sbType, req.UpdateExistingObjects)
		if err != nil {
			allErrors[getFileName(snapshot)] = err
			if req.Mode != pb.RpcObjectImportRequest_IGNORE_ERRORS {
				break
			}
			log.With(zap.String("object name", getFileName(snapshot))).Error(err)
		}
		details[snapshot.Id] = detail
	}
	return details
}

func convertType(cType string) pb.RpcObjectImportListImportResponseType {
	return pb.RpcObjectImportListImportResponseType(pb.RpcObjectImportListImportResponseType_value[cType])
}
