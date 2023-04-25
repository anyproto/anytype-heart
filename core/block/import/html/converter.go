package html

import (
	"os"
	"path/filepath"

	"github.com/textileio/go-threads/core/thread"

	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/markdown/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/threads"
)

const numberOfStages = 2 // 1 cycle to get snapshots and 1 cycle to create objects
const Name = "Html"

func init() {
	converter.RegisterFunc(New)
}

type HTML struct {
}

func New(core.Service) converter.Converter {
	return &HTML{}
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

func (h *HTML) GetSnapshots(req *pb.RpcObjectImportRequest,
	progress *process.Progress) (*converter.Response, converter.ConvertError) {
	path := h.GetParams(req)
	if len(path) == 0 {
		return nil, nil
	}
	progress.SetTotal(int64(numberOfStages * len(path)))
	progress.SetProgressMessage("Start creating snapshots from files")
	snapshots := make([]*converter.Snapshot, 0)
	for _, p := range path {
		if err := progress.TryStep(1); err != nil {
			cancellError := converter.NewFromError(p, err)
			return nil, cancellError
		}
		if filepath.Ext(p) != ".html" {
			continue
		}
		cErr := converter.NewError()
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
		tid, err := threads.ThreadCreateID(thread.AccessControlled, smartblock.SmartBlockTypePage)
		if err != nil {
			cErr.Add(p, err)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, cErr
			}
			continue
		}

		snapshot := &converter.Snapshot{
			Id:       tid.String(),
			FileName: p,
			Snapshot: sn,
		}
		snapshots = append(snapshots, snapshot)
	}
	return &converter.Response{
		Snapshots: snapshots,
	}, nil
}
