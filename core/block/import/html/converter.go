package html

import (
	"bufio"
	"fmt"
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
	cErr := converter.NewError()
	snapshots, targetObjects, cancelError := h.getSnapshots(req, progress, path, cErr)
	if !cancelError.IsEmpty() {
		return nil, cancelError
	}
	if h.shouldReturnError(req, cErr, path) {
		return nil, cErr
	}

	rootCollection := converter.NewRootCollection(h.collectionService)
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

func (h *HTML) shouldReturnError(req *pb.RpcObjectImportRequest, cErr *converter.ConvertError, path []string) bool {
	return (!cErr.IsEmpty() && req.Mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING) ||
		(cErr.IsNoObjectToImportError(len(path)))
}

func (h *HTML) getSnapshots(req *pb.RpcObjectImportRequest, progress process.Progress, path []string, cErr *converter.ConvertError) ([]*converter.Snapshot, []string, *converter.ConvertError) {
	snapshots := make([]*converter.Snapshot, 0)
	targetObjects := make([]string, 0)
	for _, p := range path {
		if err := progress.TryStep(1); err != nil {
			return nil, nil, converter.NewCancelError(err)
		}
		sn, to, err := h.handleImportPath(p, req.GetMode())
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

func (h *HTML) handleImportPath(path string, mode pb.RpcObjectImportRequestMode) ([]*converter.Snapshot, []string, error) {
	importSource := source.GetSource(path)
	if importSource == nil {
		return nil, nil, fmt.Errorf("failed to identify source: %s", path)
	}
	defer importSource.Close()
	supportedExtensions := []string{".html"}
	imageFormats := []string{".jpg", ".jpeg", ".png", ".gif", ".webp"}
	videoFormats := []string{".mp4", ".m4v", ".mov"}
	audioFormats := []string{".mp3", ".ogg", ".wav", ".m4a", ".flac"}
	pdf := []string{".pdf"}

	supportedExtensions = append(supportedExtensions, videoFormats...)
	supportedExtensions = append(supportedExtensions, imageFormats...)
	supportedExtensions = append(supportedExtensions, audioFormats...)
	supportedExtensions = append(supportedExtensions, pdf...)
	readers, err := importSource.GetFileReaders(path, supportedExtensions, nil)
	if err != nil {
		if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
			return nil, nil, err
		}
	}
	if source.CountFilesWithGivenExtension(readers, ".html") == 0 {
		return nil, nil, converter.ErrNoObjectsToImport
	}
	snapshots := make([]*converter.Snapshot, 0, len(readers))
	targetObjects := make([]string, 0, len(readers))
	for name, rc := range readers {
		if filepath.Ext(name) != ".html" {
			continue
		}
		blocks, err := h.getBlocksForSnapshot(rc, readers, path)
		if err != nil {
			if mode == pb.RpcObjectImportRequest_ALL_OR_NOTHING {
				return nil, nil, err
			}
			continue
		}
		sn, id := h.getSnapshot(blocks, name)
		snapshots = append(snapshots, sn)
		targetObjects = append(targetObjects, id)
	}
	return snapshots, targetObjects, nil
}

func (h *HTML) getBlocksForSnapshot(rc io.ReadCloser, files map[string]io.ReadCloser, path string) ([]*model.Block, error) {
	defer rc.Close()
	b, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	blocks, _, err := anymark.HTMLToBlocks(b)
	for _, block := range blocks {
		if block.GetFile() != nil {
			if newFileName, _, err := h.provideFileName(block.GetFile().GetName(), files, path); err == nil {
				block.GetFile().Name = newFileName
			} else {
				log.Errorf("failed to update file block with new file name: %v", oserror.TransformError(err))
			}
		}
		if block.GetText() != nil && block.GetText().Marks != nil && len(block.GetText().Marks.Marks) > 0 {
			h.updateFilesInLinks(block, files, path)
		}
	}
	return blocks, nil
}

func (h *HTML) updateFilesInLinks(block *model.Block, files map[string]io.ReadCloser, path string) {
	marks := block.GetText().GetMarks().GetMarks()
	for _, mark := range marks {
		if mark.Type == model.BlockContentTextMark_Link {
			var (
				err             error
				newFileName     string
				createFileBlock bool
			)
			if newFileName, createFileBlock, err = h.provideFileName(mark.Param, files, path); err == nil {
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

func (h *HTML) provideFileName(fileName string, files map[string]io.ReadCloser, path string) (string, bool, error) {
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
	if rc, ok := files[fileName]; ok {
		tempFile, err := h.extractFileFromArchiveToTempDirectory(fileName, rc)
		if err != nil {
			return "", false, oserror.TransformError(err)
		}
		createFileBlock = true
		return tempFile, createFileBlock, nil
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
