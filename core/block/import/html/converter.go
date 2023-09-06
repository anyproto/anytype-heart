package html

import (
	"bufio"
	"errors"
	"io"
	"os"
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
	allErrors := converter.NewError()
	snapshots, targetObjects := h.getSnapshots(req, progress, path, allErrors)
	if h.shouldReturnError(req, allErrors, path) {
		return nil, allErrors
	}
	rootCollection := converter.NewRootCollection(h.collectionService)
	rootCollectionSnapshot, err := rootCollection.MakeRootCollection(rootCollectionName, targetObjects)
	if err != nil {
		allErrors.Add(err)
		if req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
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

func (h *HTML) shouldReturnError(req *pb.RpcObjectImportRequest, cErr *converter.ConvertError, path []string) bool {
	return (!cErr.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING) ||
		(cErr.IsNoObjectToImportError(len(path))) ||
		errors.Is(cErr.GetResultError(pb.RpcObjectImportRequest_Html), converter.ErrCancel)
}

func (h *HTML) getSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, path []string, allErrors *converter.ConvertError) ([]*converter.Snapshot, []string) {
	snapshots := make([]*converter.Snapshot, 0)
	targetObjects := make([]string, 0)
	for _, p := range path {
		if err := progress.TryStep(1); err != nil {
			allErrors.Add(converter.ErrCancel)
			return nil, nil
		}
		sn, to := h.handleImportPath(p, req.GetMode(), allErrors)
		if h.shouldReturnError(req, allErrors, path) {
			return nil, nil
		}
		snapshots = append(snapshots, sn...)
		targetObjects = append(targetObjects, to...)
	}
	return snapshots, targetObjects
}

func (h *HTML) handleImportPath(path string, mode pb.RpcObjectImportRequestMode, allErrors *converter.ConvertError) ([]*converter.Snapshot, []string) {
	importSource := source.GetSource(path)
	defer importSource.Close()
	err := importSource.Initialize(path)
	if err != nil {
		allErrors.Add(err)
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil
		}
	}
	var numberOfFiles int
	if numberOfFiles = importSource.CountFilesWithGivenExtensions([]string{".html"}); numberOfFiles == 0 {
		allErrors.Add(converter.ErrNoObjectsToImport)
		return nil, nil
	}
	return h.getSnapshotsAndRootObjects(path, mode, allErrors, numberOfFiles, importSource)
}

func (h *HTML) getSnapshotsAndRootObjects(path string,
	mode pb.RpcObjectImportRequestMode,
	allErrors *converter.ConvertError,
	numberOfFiles int,
	importSource source.Source) ([]*converter.Snapshot, []string) {
	snapshots := make([]*converter.Snapshot, 0, numberOfFiles)
	rootObjects := make([]string, 0, numberOfFiles)
	if iterateErr := importSource.Iterate(func(fileName string, fileReader io.ReadCloser) (stop bool) {
		if filepath.Ext(fileName) != ".html" {
			return false
		}
		blocks, err := h.getBlocksForSnapshot(fileReader, importSource, path)
		if err != nil {
			allErrors.Add(err)
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
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
			if newFileName, _, err := h.provideFileName(block.GetFile().GetName(), filesSource, path); err == nil {
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
			if newFileName, createFileBlock, err = h.provideFileName(mark.Param, filesSource, path); err == nil {
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
		Details:     converter.GetCommonDetails(p, "", ""),
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

func (h *HTML) provideFileName(fileName string, filesSource source.Source, path string) (string, bool, error) {
	if strings.HasPrefix(strings.ToLower(fileName), "http://") || strings.HasPrefix(strings.ToLower(fileName), "https://") {
		return fileName, false, nil
	}
	var createFileBlock bool
	// first try to check if file exist on local machine
	absolutePath := fileName
	if !filepath.IsAbs(fileName) {
		absolutePath = filepath.Join(path, fileName)
	}
	if _, err := os.Stat(absolutePath); err == nil {
		createFileBlock = true
		return absolutePath, createFileBlock, nil
	}
	// second case for archive, when file is inside zip archive
	if handlerError := filesSource.ProcessFile(fileName, func(fileReader io.ReadCloser) error {
		var err error
		fileName, err = h.extractFileFromArchiveToTempDirectory(fileName, fileReader)
		if err != nil {
			return oserror.TransformError(err)
		}
		createFileBlock = true
		return nil
	}); handlerError != nil {
		return "", false, handlerError
	}
	return fileName, createFileBlock, nil
}

func (h *HTML) extractFileFromArchiveToTempDirectory(fileName string, rc io.ReadCloser) (string, error) {
	tempDir := h.tempDirProvider.TempDir()
	directoryWithFile := filepath.Dir(fileName)
	if directoryWithFile != "" {
		directoryWithFile = filepath.Join(tempDir, directoryWithFile)
		if err := os.Mkdir(directoryWithFile, 0777); err != nil && !os.IsExist(err) {
			return "", err
		}
	}
	pathToTmpFile := filepath.Join(tempDir, fileName)
	tmpFile, err := os.Create(pathToTmpFile)
	if os.IsExist(err) {
		return pathToTmpFile, nil
	}
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()
	w := bufio.NewWriter(tmpFile)
	_, err = w.ReadFrom(rc)
	if err != nil {
		return "", err
	}
	if err = w.Flush(); err != nil {
		return "", err
	}
	return pathToTmpFile, nil
}
