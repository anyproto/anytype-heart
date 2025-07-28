package html

import (
	"context"
	"fmt"
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
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/anyerror"
)

const numberOfStages = 2 // 1 cycle to get snapshots and 1 cycle to create objects
const (
	Name               = "Html"
	rootCollectionName = "HTML Import"
)

var log = logging.Logger("import-html")

type HTML struct {
	collectionService *collection.Service
	tempDirProvider   core.TempDirProvider
}

func New(collectionService *collection.Service, tempDirProvider core.TempDirProvider) common.Converter {
	return &HTML{
		collectionService: collectionService,
		tempDirProvider:   tempDirProvider,
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

func (h *HTML) GetSnapshots(ctx context.Context, req *pb.RpcObjectImportRequest, progress process.Progress) (*common.Response, *common.ConvertError) {
	path := h.GetParams(req)
	if len(path) == 0 {
		return nil, nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")
	allErrors := common.NewError(req.Mode)
	snapshots, targetObjects := h.getSnapshots(req, progress, path, allErrors)
	if allErrors.ShouldAbortImport(len(path), req.Type) {
		return nil, allErrors
	}
	rootCollection := common.NewImportCollection(h.collectionService)
	settings := common.NewImportCollectionSetting(
		common.WithCollectionName(rootCollectionName),
		common.WithTargetObjects(targetObjects),
		common.WithAddDate(),
		common.WithRelations(),
	)
	rootCollectionSnapshot, err := rootCollection.MakeImportCollection(settings)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(len(path), req.Type) {
			return nil, allErrors
		}
	}
	var rootCollectionID string
	if rootCollectionSnapshot != nil {
		snapshots = append(snapshots, rootCollectionSnapshot)
		rootCollectionID = rootCollectionSnapshot.Id
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

func (h *HTML) getSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, path []string, allErrors *common.ConvertError) ([]*common.Snapshot, []string) {
	snapshots := make([]*common.Snapshot, 0)
	targetObjects := make([]string, 0)
	for _, p := range path {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(common.ErrCancel)
			return nil, nil
		}
		sn, to := h.handleImportPath(p, allErrors)
		if allErrors.ShouldAbortImport(len(path), req.Type) {
			return nil, nil
		}
		snapshots = append(snapshots, sn...)
		targetObjects = append(targetObjects, to...)
	}
	return snapshots, targetObjects
}

func (h *HTML) handleImportPath(path string, allErrors *common.ConvertError) ([]*common.Snapshot, []string) {
	importSource := source.GetSource(path)
	defer importSource.Close()
	err := importSource.Initialize(path)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(len(path), model.Import_Html) {
			return nil, nil
		}
	}
	var numberOfFiles int
	if numberOfFiles = importSource.CountFilesWithGivenExtensions([]string{".html"}); numberOfFiles == 0 {
		allErrors.Add(common.ErrorBySourceType(importSource))
		return nil, nil
	}
	return h.getSnapshotsAndRootObjects(path, allErrors, numberOfFiles, importSource)
}

func (h *HTML) getSnapshotsAndRootObjects(path string,
	allErrors *common.ConvertError,
	numberOfFiles int,
	importSource source.Source,
) ([]*common.Snapshot, []string) {
	snapshots := make([]*common.Snapshot, 0, numberOfFiles)
	rootObjects := make([]string, 0, numberOfFiles)
	if iterateErr := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) (isContinue bool) {
		if filepath.Ext(fileName) != ".html" {
			return true
		}
		blocks, err := h.getBlocksForSnapshot(fileReader, importSource, path)
		if err != nil {
			allErrors.Add(err)
			if allErrors.ShouldAbortImport(len(path), model.Import_Html) {
				return false
			}
		}
		sn, id := h.getSnapshot(blocks, fileName)
		snapshots = append(snapshots, sn)
		rootObjects = append(rootObjects, id)
		return true
	}); iterateErr != nil {
		allErrors.Add(iterateErr)
	}
	return snapshots, rootObjects
}

func (h *HTML) getBlocksForSnapshot(rc io.ReadCloser, filesSource source.Source, path string) ([]*model.Block, error) {
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	blocks, _, err := anymark.HTMLToBlocks(b, "")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", common.ErrWrongHTMLFormat, err.Error())
	}
	for _, block := range blocks {
		if block.GetFile() != nil {
			if newFileName, _, err := common.ProvideFileName(block.GetFile().GetName(), filesSource, path, h.tempDirProvider); err == nil {
				block.GetFile().Name = newFileName
			} else {
				log.Errorf("failed to update file block with new file name: %v", anyerror.CleanupError(err))
			}
		}
		if block.GetText() != nil && block.GetText().Marks != nil && len(block.GetText().Marks.Marks) > 0 {
			h.updateFilesInLinks(block, filesSource, path)
		}
	}
	return blocks, nil
}

func (h *HTML) updateFilesInLinks(block *model.Block, filesSource source.Source, path string) {
	marks := block.GetText().GetMarks().GetMarks()
	for _, mark := range marks {
		if mark.Type == model.BlockContentTextMark_Link {
			var (
				err             error
				newFileName     string
				createFileBlock bool
			)
			if newFileName, createFileBlock, err = common.ProvideFileName(mark.Param, filesSource, path, h.tempDirProvider); err == nil {
				mark.Param = newFileName
				if createFileBlock {
					block.Content = anymark.ConvertTextToFile(mark.Param)
					break
				}
				continue
			}
			log.Errorf("failed to update link block with new file name: %v", anyerror.CleanupError(err))
		}
	}
}

func (h *HTML) getSnapshot(blocks []*model.Block, p string) (*common.Snapshot, string) {
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
