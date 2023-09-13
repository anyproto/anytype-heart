package txt

import (
	"fmt"
	"io"

	"github.com/gogo/protobuf/types"
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
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

func (t *TXT) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, importID string) (*converter.Response, *converter.ConvertError) {
	paths := t.GetParams(req)
	if len(paths) == 0 {
		return nil, nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")
	cErr := converter.NewError()
	snapshots, targetObjects, cancelError := t.getSnapshots(req, progress, paths, cErr, importID)
	if !cancelError.IsEmpty() {
		return nil, cancelError
	}
	if (!cErr.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING) || cErr.IsNoObjectToImportError(len(paths)) {
		return nil, cErr
	}
	rootCollection := converter.NewRootCollection(t.service)
	rootCol, err := rootCollection.MakeRootCollection(rootCollectionName, targetObjects)
	if err != nil {
		cErr.Add(err)
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

func (t *TXT) getSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, paths []string, cErr *converter.ConvertError, importID string) ([]*converter.Snapshot, []string, *converter.ConvertError) {
	snapshots := make([]*converter.Snapshot, 0)
	targetObjects := make([]string, 0)
	for _, p := range paths {
		if err := progress.TryStep(1); err != nil {
			return nil, nil, converter.NewCancelError(err)
		}
		sn, to, err := t.handleImportPath(p, req.GetMode(), importID)
		if err != nil {
			cErr.Add(err)
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

func (t *TXT) handleImportPath(p string, mode pb.RpcObjectImportRequestMode, importID string) ([]*converter.Snapshot, []string, error) {
	importSource := source.GetSource(p)
	if importSource == nil {
		return nil, nil, fmt.Errorf("failed to identify source: %s", p)
	}

	defer importSource.Close()
	readers, err := importSource.GetFileReaders(p, []string{".txt"}, nil)
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
		blocks, err := t.getBlocksForSnapshot(rc)
		if err != nil {
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, err
			}
			continue
		}
		sn, id := t.getSnapshot(blocks, name, importID)
		snapshots = append(snapshots, sn)
		targetObjects = append(targetObjects, id)
	}
	return snapshots, targetObjects, nil
}

func (t *TXT) getBlocksForSnapshot(rc io.ReadCloser) ([]*model.Block, error) {
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

func (t *TXT) getSnapshot(blocks []*model.Block, p string, importID string) (*converter.Snapshot, string) {
	details := t.provideDetails(p, importID)
	sn := &model.SmartBlockSnapshotBase{
		Blocks:      blocks,
		Details:     details,
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

func (t *TXT) provideDetails(p string, importID string) *types.Struct {
	details := converter.GetCommonDetails(p, "", "", model.ObjectType_basic)
	details.Fields[bundle.RelationKeyImportID.String()] = pbtypes.String(importID)
	return details
}
