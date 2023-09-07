package txt

import (
	"errors"
	"io"
	"path/filepath"

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

func (t *TXT) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, *converter.ConvertError) {
	paths := t.GetParams(req)
	if len(paths) == 0 {
		return nil, nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")
	allErrors := converter.NewError()
	snapshots, targetObjects := t.getSnapshots(req, progress, paths, allErrors)
	if t.shouldReturnError(req, allErrors, paths) {
		return nil, allErrors
	}
	rootCollection := converter.NewRootCollection(t.service)
	rootCol, err := rootCollection.MakeRootCollection(rootCollectionName, targetObjects)
	if err != nil {
		allErrors.Add(err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, allErrors
		}
	}
	if rootCol != nil {
		snapshots = append(snapshots, rootCol)
	}
	progress.SetTotal(int64(numberOfStages * len(snapshots)))
	if allErrors.IsEmpty() {
		return &converter.Response{Snapshots: snapshots}, nil
	}
	return &converter.Response{
		Snapshots: snapshots,
	}, allErrors
}

func (t *TXT) getSnapshots(req *pb.RpcObjectImportRequest,
	progress process.Progress,
	paths []string,
	allErrors *converter.ConvertError) ([]*converter.Snapshot, []string) {
	snapshots := make([]*converter.Snapshot, 0)
	targetObjects := make([]string, 0)
	for _, p := range paths {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(converter.ErrCancel)
			return nil, nil
		}
		sn, to := t.handleImportPath(p, req.GetMode(), allErrors)
		if !allErrors.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil
		}
		snapshots = append(snapshots, sn...)
		targetObjects = append(targetObjects, to...)
	}
	return snapshots, targetObjects
}

func (t *TXT) handleImportPath(p string, mode pb.RpcObjectImportRequestMode, allErrors *converter.ConvertError) ([]*converter.Snapshot, []string) {
	importSource := source.GetSource(p)
	defer importSource.Close()
	err := importSource.Initialize(p)
	if err != nil {
		allErrors.Add(err)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil
		}
	}
	var numberOfFiles int
	if numberOfFiles = importSource.CountFilesWithGivenExtensions([]string{".txt"}); numberOfFiles == 0 {
		allErrors.Add(converter.ErrNoObjectsToImport)
		return nil, nil
	}
	snapshots := make([]*converter.Snapshot, 0, numberOfFiles)
	targetObjects := make([]string, 0, numberOfFiles)
	iterateErr := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) (stop bool) {
		if filepath.Ext(fileName) != ".txt" {
			return false
		}
		var blocks []*model.Block
		blocks, err = t.getBlocksForSnapshot(fileReader)
		if err != nil {
			allErrors.Add(err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return true
			}
		}
		sn, id := t.getSnapshot(blocks, fileName)
		snapshots = append(snapshots, sn)
		targetObjects = append(targetObjects, id)
		return false
	})
	if iterateErr != nil {
		allErrors.Add(iterateErr)
	}
	return snapshots, targetObjects
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

func (t *TXT) getSnapshot(blocks []*model.Block, p string) (*converter.Snapshot, string) {
	sn := &model.SmartBlockSnapshotBase{
		Blocks:      blocks,
		Details:     converter.GetCommonDetails(p, "", "", model.ObjectType_basic),
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

func (t *TXT) shouldReturnError(req *pb.RpcObjectImportRequest, cErr *converter.ConvertError, path []string) bool {
	return (!cErr.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING) ||
		(cErr.IsNoObjectToImportError(len(path))) ||
		errors.Is(cErr.GetResultError(pb.RpcObjectImportRequest_Txt), converter.ErrCancel)
}
