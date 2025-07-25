package txt

import (
	"context"
	"io"
	"path/filepath"

	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/collection"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/core/block/import/markdown/anymark"
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

func New(service *collection.Service) common.Converter {
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

func (t *TXT) GetSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*common.Response, *common.ConvertError) {
	paths := t.GetParams(req)
	if len(paths) == 0 {
		return nil, nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")
	allErrors := common.NewError(req.Mode)
	snapshots, targetObjects := t.getSnapshots(req, progress, paths, allErrors)
	if allErrors.ShouldAbortImport(len(paths), req.Type) {
		return nil, allErrors
	}
	rootCollection := common.NewImportCollection(t.service)
	settings := common.NewImportCollectionSetting(
		common.WithCollectionName(rootCollectionName),
		common.WithTargetObjects(targetObjects),
		common.WithRelations(),
		common.WithAddDate(),
	)
	rootCol, err := rootCollection.MakeImportCollection(settings)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(len(paths), req.Type) {
			return nil, allErrors
		}
	}
	var rootCollectionID string
	if rootCol != nil {
		snapshots = append(snapshots, rootCol)
		rootCollectionID = rootCol.Id
	}
	progress.SetTotal(int64(numberOfStages * len(snapshots)))
	if allErrors.IsEmpty() {
		return &common.Response{Snapshots: snapshots, RootObjectID: rootCollectionID, RootObjectWidgetType: model.BlockContentWidget_CompactList}, nil
	}
	return &common.Response{
		Snapshots:            snapshots,
		RootObjectID:         rootCollectionID,
		RootObjectWidgetType: model.BlockContentWidget_CompactList,
	}, allErrors
}

func (t *TXT) getSnapshots(req *pb.RpcObjectImportRequest,
	progress process.Progress,
	paths []string,
	allErrors *common.ConvertError,
) ([]*common.Snapshot, []string) {
	snapshots := make([]*common.Snapshot, 0)
	targetObjects := make([]string, 0)
	for _, p := range paths {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return nil, nil
		}
		sn, to := t.handleImportPath(p, len(paths), allErrors)
		if allErrors.ShouldAbortImport(len(paths), req.Type) {
			return nil, nil
		}
		snapshots = append(snapshots, sn...)
		targetObjects = append(targetObjects, to...)
	}
	return snapshots, targetObjects
}

func (t *TXT) handleImportPath(p string, pathsCount int, allErrors *common.ConvertError) ([]*common.Snapshot, []string) {
	importSource := source.GetSource(p)
	defer importSource.Close()
	err := importSource.Initialize(p)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(pathsCount, model.Import_Txt) {
			return nil, nil
		}
	}
	var numberOfFiles int
	if numberOfFiles = importSource.CountFilesWithGivenExtensions([]string{".txt"}); numberOfFiles == 0 {
		allErrors.Add(common.ErrorBySourceType(importSource))
		return nil, nil
	}
	snapshots := make([]*common.Snapshot, 0, numberOfFiles)
	targetObjects := make([]string, 0, numberOfFiles)
	iterateErr := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) (isContinue bool) {
		if filepath.Ext(fileName) != ".txt" {
			return true
		}
		var blocks []*model.Block
		blocks, err = t.getBlocksForSnapshot(fileReader)
		if err != nil {
			allErrors.Add(err)
			if allErrors.ShouldAbortImport(pathsCount, model.Import_Txt) {
				return false
			}
		}
		sn, id := t.getSnapshot(blocks, fileName)
		snapshots = append(snapshots, sn)
		targetObjects = append(targetObjects, id)
		return true
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

func (t *TXT) getSnapshot(blocks []*model.Block, p string) (*common.Snapshot, string) {
	sn := &common.StateSnapshot{
		Blocks:      blocks,
		Details:     common.GetCommonDetails(p, "", "", model.ObjectType_basic),
		ObjectTypes: []string{bundle.TypeKeyPage.String()},
	}

	snapshot := &common.Snapshot{
		Id:       uuid.New().String(),
		FileName: p,
		Snapshot: &common.SnapshotModel{
			SbType: smartblock.SmartBlockTypePage,
			Data:   sn,
		},
	}
	return snapshot, snapshot.Id
}
