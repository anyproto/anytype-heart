package txt

import (
	"fmt"
	"io"

	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/collection"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/converter"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/markdown/anymark"
	"github.com/anytypeio/go-anytype-middleware/core/block/import/source"
	"github.com/anytypeio/go-anytype-middleware/core/block/process"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const numberOfStages = 2 // 1 cycle to get snapshots and 1 cycle to create objects
const (
	Name               = "Txt"
	rootCollectionName = "TXT Import"
)

type TXT struct {
	service *collection.Service
}

func New(service *collection.Service) converter.Converter {
	return &TXT{service: service}
}

func (t *TXT) Name() string {
	return Name
}

func (t *TXT) GetParams(req *pb.RpcObjectImportRequest) []string {
	if p := req.GetTxtParams(); p != nil {
		return p.Path
	}

	return nil
}

func (t *TXT) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, converter.ConvertError) {
	paths := t.GetParams(req)
	if len(paths) == 0 {
		return nil, nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")
	cErr := converter.NewError()
	snapshots, targetObjects, cancelError := t.getSnapshotsForImport(req, progress, paths, cErr)
	if !cancelError.IsEmpty() {
		return nil, cancelError
	}
	if !cErr.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		return nil, cErr
	}
	rootCollection := converter.NewRootCollection(t.service)
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
	progress.SetTotal(int64(numberOfStages * len(snapshots)))
	if cErr.IsEmpty() {
		return &converter.Response{Snapshots: snapshots}, nil
	}
	return &converter.Response{
		Snapshots: snapshots,
	}, cErr
}

func (t *TXT) getSnapshotsForImport(req *pb.RpcObjectImportRequest,
	progress process.Progress,
	paths []string,
	cErr converter.ConvertError) ([]*converter.Snapshot, []string, converter.ConvertError) {
	snapshots := make([]*converter.Snapshot, 0)
	targetObjects := make([]string, 0)
	for _, p := range paths {
		if err := progress.TryStep(1); err != nil {
			cancelError := converter.NewFromError(p, err)
			return nil, nil, cancelError
		}
		sn, to, err := t.handleImportPath(p, req.GetMode())
		if err != nil {
			cErr.Add(p, err)
			if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, nil
			}
			continue
		}
		snapshots = append(snapshots, sn...)
		targetObjects = append(targetObjects, to...)
	}
	return snapshots, targetObjects, nil
}

func (t *TXT) handleImportPath(p string, mode pb.RpcObjectImportRequestMode) ([]*converter.Snapshot, []string, error) {
	s := source.GetSource(p)
	if s == nil {
		return nil, nil, fmt.Errorf("failed to identify source: %s", p)
	}

	readers, err := s.GetFileReaders(p, []string{".txt"})
	if err != nil {
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, err
		}
	}
	snapshots := make([]*converter.Snapshot, 0, len(readers))
	targetObjects := make([]string, 0, len(readers))
	for name, rc := range readers {
		blocks, err := t.getBlocksForFile(rc)
		if err != nil {
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, err
			}
			continue
		}
		sn, id := t.getSnapshot(blocks, name)
		snapshots = append(snapshots, sn)
		targetObjects = append(targetObjects, id)
	}
	return snapshots, targetObjects, nil
}

func (t *TXT) getBlocksForFile(rc io.ReadCloser) ([]*model.Block, error) {
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	blocks, _, err := anymark.MarkdownToBlocks(b, "", []string{})
	if err != nil {
		return nil, err
	}
	return blocks, nil
}

func (t *TXT) getSnapshot(blocks []*model.Block, p string) (*converter.Snapshot, string) {
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
	return snapshot, snapshot.Id
}
