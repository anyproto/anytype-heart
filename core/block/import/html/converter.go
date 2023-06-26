package html

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
	"github.com/anyproto/anytype-heart/core/block/import/source"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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

func (h *HTML) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, converter.ConvertError) {
	path := h.GetParams(req)
	if len(path) == 0 {
		return nil, nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")
	cErr := converter.NewError()
	snapshots, targetObjects, cancelError := h.getSnapshotsForImport(req, progress, path, cErr)
	if !cancelError.IsEmpty() {
		return nil, cancelError
	}
	if !cErr.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
		return nil, cErr
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

	progress.SetTotal(int64(numberOfStages * len(snapshots)))
	if cErr.IsEmpty() {
		return &converter.Response{Snapshots: snapshots}, nil
	}

	return &converter.Response{
		Snapshots: snapshots,
	}, cErr
}

func (h *HTML) getSnapshotsForImport(req *pb.RpcObjectImportRequest,
	progress process.Progress,
	path []string,
	cErr converter.ConvertError) ([]*converter.Snapshot, []string, converter.ConvertError) {
	snapshots := make([]*converter.Snapshot, 0)
	targetObjects := make([]string, 0)
	for _, p := range path {
		if err := progress.TryStep(1); err != nil {
			return nil, nil, converter.NewCancelError(p, err)
		}
		sn, to, err := h.handleImportPath(p, req.GetMode())
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

func (h *HTML) handleImportPath(path string, mode pb.RpcObjectImportRequestMode) ([]*converter.Snapshot, []string, error) {
	s := source.GetSource(path)
	if s == nil {
		return nil, nil, fmt.Errorf("failed to identify source: %s", path)
	}

	readers, err := s.GetFileReaders(path, []string{".html"})
	if err != nil {
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, err
		}
	}
	if len(readers) == 0 {
		return nil, nil, converter.ErrNoObjectsToImport
	}
	snapshots := make([]*converter.Snapshot, 0, len(readers))
	targetObjects := make([]string, 0, len(readers))
	for name, rc := range readers {
		blocks, err := h.getBlocksForFile(rc)
		if err != nil {
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, err
			}
			continue
		}
		sn, id := h.getSnapshot(blocks, path, name)
		snapshots = append(snapshots, sn)
		targetObjects = append(targetObjects, id)
	}
	return snapshots, targetObjects, nil
}

func (h *HTML) getBlocksForFile(rc io.ReadCloser) ([]*model.Block, error) {
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	blocks, _, err := anymark.HTMLToBlocks(b)
	if err != nil {
		return nil, err
	}
	return blocks, nil
}

func (h *HTML) getSnapshot(blocks []*model.Block, path, shortFileName string) (*converter.Snapshot, string) {
	name := strings.TrimSuffix(filepath.Base(shortFileName), filepath.Ext(shortFileName))
	details := converter.GetCommonDetails(name, "", converter.GetSourceDetail(shortFileName))
	sn := &model.SmartBlockSnapshotBase{
		Blocks:      blocks,
		Details:     details,
		ObjectTypes: []string{bundle.TypeKeyPage.URL()},
	}

	snapshot := &converter.Snapshot{
		Id:       uuid.New().String(),
		FileName: path,
		Snapshot: &pb.ChangeSnapshot{Data: sn},
		SbType:   smartblock.SmartBlockTypePage,
	}
	return snapshot, snapshot.Id
}
