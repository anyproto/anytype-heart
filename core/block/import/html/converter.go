package html

import (
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
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	oserror "github.com/anyproto/anytype-heart/util/os"
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

func New(collectionService *collection.Service, tempDirProvider core.TempDirProvider) converter.Converter {
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

func (h *HTML) GetSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress) (*converter.Response, *converter.ConvertError) {
	path := h.GetParams(req)
	if len(path) == 0 {
		return nil, nil
	}
	progress.SetProgressMessage("Start creating snapshots from files")
	allErrors := converter.NewError(req.Mode)
	snapshots, targetObjects := h.getSnapshots(req, progress, path, allErrors)
	if allErrors.ShouldAbortImport(len(path), req.Type) {
		return nil, allErrors
	}
	rootCollection := converter.NewRootCollection(h.collectionService)
	rootCollectionSnapshot, err := rootCollection.MakeRootCollection(rootCollectionName, targetObjects)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(len(path), req.Type) {
			return nil, allErrors
		}
	}
	if rootCollectionSnapshot != nil {
		snapshots = append(snapshots, rootCollectionSnapshot)
	}
	progress.SetTotal(int64(numberOfStages * len(snapshots)))
	if allErrors.IsEmpty() {
		return &converter.Response{Snapshots: snapshots}, nil
	}

	return &converter.Response{
		Snapshots: snapshots,
	}, allErrors
}

func (h *HTML) getSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, path []string, allErrors *converter.ConvertError) ([]*converter.Snapshot, []string) {
	snapshots := make([]*converter.Snapshot, 0)
	targetObjects := make([]string, 0)
	for _, p := range path {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(converter.ErrCancel)
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

func (h *HTML) handleImportPath(path string, allErrors *converter.ConvertError) ([]*converter.Snapshot, []string) {
	importSource := source.GetSource(path)
	defer importSource.Close()
	err := importSource.Initialize(path)
	if err != nil {
		allErrors.Add(err)
		if allErrors.ShouldAbortImport(len(path), pb.RpcObjectImportRequest_Html) {
			return nil, nil
		}
	}
	var numberOfFiles int
	if numberOfFiles = importSource.CountFilesWithGivenExtensions([]string{".html"}); numberOfFiles == 0 {
		allErrors.Add(converter.ErrNoObjectsToImport)
		return nil, nil
	}
	return h.getSnapshotsAndRootObjects(path, allErrors, numberOfFiles, importSource)
}

func (h *HTML) getSnapshotsAndRootObjects(path string,
	allErrors *converter.ConvertError,
	numberOfFiles int,
	importSource source.Source,
) ([]*converter.Snapshot, []string) {
	snapshots := make([]*converter.Snapshot, 0, numberOfFiles)
	rootObjects := make([]string, 0, numberOfFiles)
	if iterateErr := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) (stop bool) {
		if filepath.Ext(fileName) != ".html" {
			return false
		}
		blocks, err := h.getBlocksForSnapshot(fileReader, importSource, path)
		if err != nil {
			allErrors.Add(err)
			if allErrors.ShouldAbortImport(len(path), pb.RpcObjectImportRequest_Html) {
				return true
			}
		}
		sn, id := h.getSnapshot(blocks, fileName)
		snapshots = append(snapshots, sn)
		rootObjects = append(rootObjects, id)
		return false
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
	blocks, _, err := anymark.HTMLToBlocks(b)
	for _, block := range blocks {
		if block.GetFile() != nil {
			if newFileName, _, err := converter.ProvideFileName(block.GetFile().GetName(), filesSource, path, h.tempDirProvider); err == nil {
				block.GetFile().Name = newFileName
			} else {
				log.Errorf("failed to update file block with new file name: %v", oserror.TransformError(err))
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
			if newFileName, createFileBlock, err = converter.ProvideFileName(mark.Param, filesSource, path, h.tempDirProvider); err == nil {
				mark.Param = newFileName
				if createFileBlock {
					anymark.ConvertTextToFile(block)
					break
				}
				continue
			}
			log.Errorf("failed to update link block with new file name: %v", oserror.TransformError(err))
		}
	}
}

func (h *HTML) getSnapshot(blocks []*model.Block, p string) (*converter.Snapshot, string) {
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
