package html

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/markdown/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const numberOfStages = 2 // 1 cycle to get snapshots and 1 cycle to create objects
const (
	Name               = "Html"
	rootCollectionName = "HTML Import"
)

type HTML struct {
	collectionService *collection.Service
}

func New(c *collection.Service) converter.Converter {
	return &HTML{
		collectionService: c,
	}
}

func (h *HTML) Name() string {
	return Name
}

func (h *HTML) GetParams(req *pb.RpcObjectImportRequest) []string {
	if p := req.GetHtmlParams(); p != nil {
		return p.Path
	}

	return nil
}

func (h *HTML) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.IProgress) (*converter.Response, converter.ConvertError) {
	path := h.GetParams(req)
	if len(path) == 0 {
		return nil, nil
	}
	progress.SetTotal(int64(numberOfStages * len(path)))
	progress.SetProgressMessage("Start creating snapshots from files")
	snapshots := make([]*converter.Snapshot, 0)
	targetObjects := make([]string, 0, len(path))
	cErr := converter.NewError()
	for _, p := range path {
		if err := progress.TryStep(1); err != nil {
			cancellError := converter.NewFromError(p, err)
			return nil, cancellError
		}
		if filepath.Ext(p) != ".html" {
			continue
		}
		source, err := os.ReadFile(p)
		if err != nil {
			cErr.Add(p, err)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, cErr
			}
			continue
		}

		blocks, _, err := anymark.HTMLToBlocks(source)
		if err != nil {
			cErr.Add(p, err)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, cErr
			}
			continue
		}

		sn := &model.SmartBlockSnapshotBase{
			Blocks:      blocks,
			Details:     converter.GetDetails(p),
			ObjectTypes: []string{bundle.TypeKeyPage.URL()},
		}

		snapshot := &converter.Snapshot{
			Id:       uuid.New().String(),
			FileName: p,
			Snapshot: &pb.ChangeSnapshot{Data: sn},
			SbType:   smartblock.SmartBlockTypePage,
		}
		snapshots = append(snapshots, snapshot)
		targetObjects = append(targetObjects, snapshot.Id)
	}

	rootCollection := converter.NewRootCollection(h.collectionService)
	rootCol, err := rootCollection.AddObjects(rootCollectionName, targetObjects)
	if err != nil {
		cErr.Add(rootCollectionName, err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, cErr
		}
	}

	if rootCol != nil {
		snapshots = append(snapshots, rootCol)
	}

	if cErr.IsEmpty() {
		return &converter.Response{Snapshots: snapshots}, nil
	}

	return &converter.Response{
		Snapshots: snapshots,
	}, cErr
}
